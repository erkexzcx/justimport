package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// InstanceConfig holds the configuration for a single *arr instance.
type InstanceConfig struct {
	Type   string // "radarr" or "sonarr"
	URL    string
	APIKey string
}

// Config holds the application configuration parsed from environment variables.
type Config struct {
	Instances    []InstanceConfig
	PollInterval time.Duration
	DryRun       bool
}

// Load reads and validates configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	// Load Radarr instances
	radarrURL := os.Getenv("RADARR_URL")
	radarrAPIKey := os.Getenv("RADARR_API_KEY")
	if radarrURL != "" {
		cfg.Instances = append(cfg.Instances, InstanceConfig{Type: "radarr", URL: radarrURL, APIKey: radarrAPIKey})
	}
	
	for i := 1; ; i++ {
		url := os.Getenv(fmt.Sprintf("RADARR_URL_%d", i))
		if url == "" {
			break
		}
		apiKey := os.Getenv(fmt.Sprintf("RADARR_API_KEY_%d", i))
		cfg.Instances = append(cfg.Instances, InstanceConfig{Type: "radarr", URL: url, APIKey: apiKey})
	}

	// Load Sonarr instances
	sonarrURL := os.Getenv("SONARR_URL")
	sonarrAPIKey := os.Getenv("SONARR_API_KEY")
	if sonarrURL != "" {
		cfg.Instances = append(cfg.Instances, InstanceConfig{Type: "sonarr", URL: sonarrURL, APIKey: sonarrAPIKey})
	}

	for i := 1; ; i++ {
		url := os.Getenv(fmt.Sprintf("SONARR_URL_%d", i))
		if url == "" {
			break
		}
		apiKey := os.Getenv(fmt.Sprintf("SONARR_API_KEY_%d", i))
		cfg.Instances = append(cfg.Instances, InstanceConfig{Type: "sonarr", URL: url, APIKey: apiKey})
	}

	if len(cfg.Instances) == 0 {
		return nil, errors.New("at least one RADARR_URL or SONARR_URL must be set")
	}

	pollIntervalStr := os.Getenv("POLL_INTERVAL")
	if pollIntervalStr == "" {
		pollIntervalStr = "60s"
	}

	dur, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", pollIntervalStr, err)
	}

	cfg.PollInterval = dur

	dryRunStr := os.Getenv("DRY_RUN")
	if dryRunStr == "" {
		cfg.DryRun = true
	} else {
		cfg.DryRun = !strings.EqualFold(dryRunStr, "false")
	}

	return cfg, nil
}
