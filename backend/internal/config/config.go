// Package config loads Podora's runtime configuration from the environment.
package config

import "os"

// Config holds the backend's runtime settings.
type Config struct {
	// Addr is the address the HTTP server listens on (PODORA_ADDR).
	Addr string
	// Kubeconfig is an explicit kubeconfig path (PODORA_KUBECONFIG). Empty
	// means in-cluster credentials, then the standard kubeconfig lookup.
	Kubeconfig string
	// DataDir holds Podora's database: users, sessions, audit log
	// (PODORA_DATA_DIR). It must survive restarts, or everyone has to be
	// set up again.
	DataDir string
	// SetupToken presets the one-time token for creating the first
	// administrator (PODORA_SETUP_TOKEN). Empty generates a random one.
	SetupToken string
}

// Load reads configuration from environment variables, applying defaults.
func Load() Config {
	return Config{
		Addr:       env("PODORA_ADDR", ":8080"),
		Kubeconfig: os.Getenv("PODORA_KUBECONFIG"),
		DataDir:    env("PODORA_DATA_DIR", "data"),
		SetupToken: os.Getenv("PODORA_SETUP_TOKEN"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
