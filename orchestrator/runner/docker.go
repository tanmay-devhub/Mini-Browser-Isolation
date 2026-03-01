package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"go.uber.org/zap"
)

// DockerRunner spawns and manages runner containers via the Docker API.
type DockerRunner struct {
	client      *dockerclient.Client
	image       string
	networkName string
	log         *zap.Logger
}

// NewDockerRunner creates a runner backed by the local Docker daemon.
func NewDockerRunner(image, networkName string, log *zap.Logger) (*DockerRunner, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerRunner{
		client:      cli,
		image:       image,
		networkName: networkName,
		log:         log,
	}, nil
}

// SpawnResult holds the container ID and internal address of the runner.
type SpawnResult struct {
	ContainerID string
	RunnerAddr  string // container-name:port reachable within the shared Docker network
}

func getenvInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func getenvStr(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

// Spawn creates and starts a runner container for the given session.
func (d *DockerRunner) Spawn(ctx context.Context, sessionID, targetURL string) (*SpawnResult, error) {
	// Use first 8 chars of session ID to keep the name short but identifiable.
	if len(sessionID) < 8 {
		return nil, fmt.Errorf("invalid sessionID (too short): %q", sessionID)
	}
	name := "runner-" + sessionID[:8]

	// ---- Chromium stability knobs ----
	// Increase /dev/shm (Chromium often crashes with tiny shm).
	// Default: 1GB
	shmSize := getenvInt64("RUNNER_SHM_SIZE", 1024*1024*1024)

	// Pass chrome flags to runner (runner must read CHROME_FLAGS).
	// NOTE: Using --disable-dev-shm-usage means chromium uses /tmp instead of /dev/shm.
	// You can keep it or remove it once shm is large enough.
	chromeFlags := getenvStr(
		"RUNNER_CHROME_FLAGS",
		"--no-sandbox --disable-dev-shm-usage --disable-gpu --headless=new --remote-debugging-port=9222",
	)

	// Build env list
	env := []string{
		"SESSION_ID=" + sessionID,
		"TARGET_URL=" + targetURL,
		"RUNNER_PORT=8080",
	}
	if strings.TrimSpace(chromeFlags) != "" {
		env = append(env, "CHROME_FLAGS="+chromeFlags)
	}

	cfg := &container.Config{
		Image: d.image,
		Env:   env,
	}

	// 1 CPU, 512 MB RAM – conservative defaults; override later if needed.
	hostCfg := &container.HostConfig{
		Resources: container.Resources{
			NanoCPUs: 1_000_000_000,
			Memory:   512 * 1024 * 1024,
		},

		// /dev/shm size for Chromium stability
		ShmSize: shmSize,

		CapDrop: []string{"ALL"},
		CapAdd:  []string{"NET_ADMIN"},
		// Chromium's renderer/zygote processes are killed by Docker's default seccomp
		// profile (clone3, pidfd_open, etc.). Unconfine seccomp for runner containers.
		SecurityOpt: []string{"seccomp=unconfined"},
		AutoRemove:  false,
	}

	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			d.networkName: {},
		},
	}

	resp, err := d.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, name)
	if err != nil {
		return nil, fmt.Errorf("ContainerCreate: %w", err)
	}

	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("ContainerStart: %w", err)
	}

	d.log.Info("runner container started",
		zap.String("containerID", resp.ID[:12]),
		zap.String("name", name),
		zap.String("sessionId", sessionID),
		zap.Int64("shmSize", shmSize),
		zap.String("network", d.networkName),
	)

	return &SpawnResult{
		ContainerID: resp.ID,
		RunnerAddr:  fmt.Sprintf("%s:8080", name),
	}, nil
}

// Stop kills and removes the container (10-second graceful timeout then SIGKILL).
func (d *DockerRunner) Stop(ctx context.Context, containerID string) error {
	timeout := 10
	if err := d.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		d.log.Warn("ContainerStop error (may be already stopped)",
			zap.String("containerID", containerID[:12]),
			zap.Error(err))
	}

	if err := d.client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	}); err != nil {
		return fmt.Errorf("ContainerRemove: %w", err)
	}

	d.log.Info("runner container removed", zap.String("containerID", containerID[:12]))
	return nil
}

// Stats returns a lightweight CPU/mem snapshot from the Docker stats endpoint.
// Returns zeros on any error (non-fatal; used only for metrics enrichment).
func (d *DockerRunner) Stats(ctx context.Context, containerID string) (cpuPercent, memMB float64) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := d.client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil || len(raw) == 0 {
		return 0, 0
	}

	var s statsPayload
	if err := decodeStats(raw, &s); err != nil {
		return 0, 0
	}

	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemCPUUsage - s.PreCPUStats.SystemCPUUsage)
	numCPU := float64(s.CPUStats.OnlineCPUs)

	if sysDelta > 0 && numCPU > 0 {
		cpuPercent = (cpuDelta / sysDelta) * numCPU * 100.0
	}
	memMB = float64(s.MemoryStats.Usage) / (1024 * 1024)
	return cpuPercent, memMB
}