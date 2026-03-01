package webrtc

import (
	"encoding/json"
	"fmt"

	pionwebrtc "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

// PeerConnection wraps Pion's WebRTC peer connection with the video track and
// input data channel already configured.
type PeerConnection struct {
	pc        *pionwebrtc.PeerConnection
	VideoTrack *pionwebrtc.TrackLocalStaticSample
	InputCh   chan []byte // input events received over DataChannel
	log       *zap.Logger
}

// NewPeerConnection creates a Pion PeerConnection with a VP8 video track and an
// "input" data channel listener. iceServers is passed from env/config.
func NewPeerConnection(iceServers []pionwebrtc.ICEServer, log *zap.Logger) (*PeerConnection, error) {
	api := pionwebrtc.NewAPI()

	cfg := pionwebrtc.Configuration{
		ICEServers: iceServers,
	}

	pc, err := api.NewPeerConnection(cfg)
	if err != nil {
		return nil, fmt.Errorf("new peer connection: %w", err)
	}

	// Add a VP8 video track for frame streaming.
	videoTrack, err := pionwebrtc.NewTrackLocalStaticSample(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeVP8},
		"video",
		"browser-stream",
	)
	if err != nil {
		return nil, fmt.Errorf("new video track: %w", err)
	}

	if _, err := pc.AddTrack(videoTrack); err != nil {
		return nil, fmt.Errorf("add video track: %w", err)
	}

	inputCh := make(chan []byte, 128)

	// Listen for the "input" data channel opened by the remote peer (client).
	pc.OnDataChannel(func(dc *pionwebrtc.DataChannel) {
		if dc.Label() != "input" {
			return
		}
		log.Info("input data channel opened")
		dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			select {
			case inputCh <- msg.Data:
			default:
				// Drop if the channel is full (backpressure; shouldn't happen in practice).
			}
		})
	})

	// Log ICE connection state changes.
	pc.OnICEConnectionStateChange(func(state pionwebrtc.ICEConnectionState) {
		log.Info("ICE connection state", zap.String("state", state.String()))
	})

	pc.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
		log.Info("peer connection state", zap.String("state", state.String()))
	})

	return &PeerConnection{
		pc:         pc,
		VideoTrack: videoTrack,
		InputCh:    inputCh,
		log:        log,
	}, nil
}

// SDPOffer is the JSON payload sent by the client.
type SDPOffer struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// SDPAnswer is the JSON payload returned to the client.
type SDPAnswer struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// HandleOffer processes an SDP offer and returns an SDP answer.
func (p *PeerConnection) HandleOffer(offerJSON []byte) ([]byte, error) {
	var offer SDPOffer
	if err := json.Unmarshal(offerJSON, &offer); err != nil {
		return nil, fmt.Errorf("parse offer: %w", err)
	}

	sdpOffer := pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}

	if err := p.pc.SetRemoteDescription(sdpOffer); err != nil {
		return nil, fmt.Errorf("SetRemoteDescription: %w", err)
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return nil, fmt.Errorf("CreateAnswer: %w", err)
	}

	// Block until ICE gathering is complete so the answer contains all candidates.
	gatherComplete := pionwebrtc.GatheringCompletePromise(p.pc)
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return nil, fmt.Errorf("SetLocalDescription: %w", err)
	}
	<-gatherComplete

	local := p.pc.LocalDescription()
	resp := SDPAnswer{Type: local.Type.String(), SDP: local.SDP}
	return json.Marshal(resp)
}

// Close shuts down the peer connection.
func (p *PeerConnection) Close() error {
	return p.pc.Close()
}
