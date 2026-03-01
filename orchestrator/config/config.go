package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port            string
	MaxSessions     int
	SessionTimeout  time.Duration
	RunnerImage     string
	RunnerNetwork   string
	DockerHost      string
	TURNEnabled     bool
	TURNHost        string
	TURNPort        string
	TURNUsername    string
	TURNCredential  string
	STUNHost        string
	WebRTCTimeoutSec int
	FallbackFPS     int
	LogLevel        string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8090"),
		MaxSessions:     getEnvInt("MAX_SESSIONS", 10),
		SessionTimeout:  getEnvDuration("SESSION_TIMEOUT", 30*time.Minute),
		RunnerImage:     getEnv("RUNNER_IMAGE", "mini-browser-runner:latest"),
		RunnerNetwork:   getEnv("RUNNER_NETWORK", "mini-browser-net"),
		DockerHost:      getEnv("DOCKER_HOST", "unix:///var/run/docker.sock"),
		TURNEnabled:     getEnvBool("TURN_ENABLED", false),
		TURNHost:        getEnv("TURN_HOST", "coturn"),
		TURNPort:        getEnv("TURN_PORT", "3478"),
		TURNUsername:    getEnv("TURN_USERNAME", "user"),
		TURNCredential:  getEnv("TURN_CREDENTIAL", "password"),
		STUNHost:        getEnv("STUN_HOST", "stun:stun.l.google.com:19302"),
		WebRTCTimeoutSec: getEnvInt("WEBRTC_TIMEOUT_SEC", 15),
		FallbackFPS:     getEnvInt("FALLBACK_FPS", 5),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
