package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port                    int           `yaml:"port"`
	DataDir                 string        `yaml:"data_dir"`
	ReadTimeout             time.Duration `yaml:"read_timeout"`
	WriteTimeout            time.Duration `yaml:"write_timeout"`
	IdleTimeout             time.Duration `yaml:"idle_timeout"`
	LogLevel                string        `yaml:"log_level"`
	ReplayWindowHours       int           `yaml:"replay_window_hours"`
	AttestationTimeoutHours int           `yaml:"attestation_timeout_hours"`
	MaxRetries              int           `yaml:"max_retries"`
	SubscriberBuffer        int           `yaml:"subscriber_buffer"`
	EventPruneInterval      time.Duration `yaml:"event_prune_interval"`
}

func Default() Config {
	return Config{
		Port:                    56058,
		DataDir:                 "./data",
		ReadTimeout:             30 * time.Second,
		WriteTimeout:            30 * time.Second,
		IdleTimeout:             120 * time.Second,
		LogLevel:                "info",
		ReplayWindowHours:       72,
		AttestationTimeoutHours: 48,
		MaxRetries:              5,
		SubscriberBuffer:        256,
		EventPruneInterval:      1 * time.Hour,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return cfg, fmt.Errorf("read config file: %w", err)
			}
		} else {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return cfg, fmt.Errorf("parse config: %w", err)
			}
		}
	}
	applyEnvOverrides(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("VOLTFORGE_PORT"); v != "" {
		var port int
		fmt.Sscanf(v, "%d", &port)
		if port > 0 {
			cfg.Port = port
		}
	}
	if v := os.Getenv("VOLTFORGE_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("VOLTFORGE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("VOLTFORGE_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ReadTimeout = d
		}
	}
	if v := os.Getenv("VOLTFORGE_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.WriteTimeout = d
		}
	}
	if v := os.Getenv("VOLTFORGE_REPLAY_WINDOW_HOURS"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.ReplayWindowHours)
	}
	if v := os.Getenv("VOLTFORGE_ATTESTATION_TIMEOUT_HOURS"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.AttestationTimeoutHours)
	}
}

func (c Config) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}
	if c.ReplayWindowHours <= 0 {
		return fmt.Errorf("replay_window_hours must be positive")
	}
	if c.AttestationTimeoutHours <= 0 {
		return fmt.Errorf("attestation_timeout_hours must be positive")
	}
	return nil
}

func (c Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

func (c Config) ReplayWindow() time.Duration {
	return time.Duration(c.ReplayWindowHours) * time.Hour
}

func (c Config) AttestationTimeout() time.Duration {
	return time.Duration(c.AttestationTimeoutHours) * time.Hour
}
