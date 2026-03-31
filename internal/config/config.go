package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

const DefaultPath = "/etc/davit/davit.toml"

// Config is the top-level configuration structure backed by /etc/davit/davit.toml.
type Config struct {
	Server  ServerConfig  `toml:"server"`
	Caddy   CaddyConfig   `toml:"caddy"`
	Daemon  DaemonConfig  `toml:"daemon"`
	Logging LoggingConfig `toml:"logging"`
	Ports   PortsConfig   `toml:"ports"`
}

type ServerConfig struct {
	Hostname   string `toml:"hostname"`
	Timezone   string `toml:"timezone"`
	AdminEmail string `toml:"admin_email"`
}

type CaddyConfig struct {
	AdminAPI string `toml:"admin_api"`
}

type DaemonConfig struct {
	PollDefaultInterval int    `toml:"poll_default_interval"`
	WebhookListenAddr   string `toml:"webhook_listen_addr"`
}

type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

type PortsConfig struct {
	AutoAssignRangeStart int `toml:"auto_assign_range_start"`
	AutoAssignRangeEnd   int `toml:"auto_assign_range_end"`
}

// Default returns a Config populated with the spec-defined defaults.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Timezone: "UTC",
		},
		Caddy: CaddyConfig{
			AdminAPI: "http://localhost:2019",
		},
		Daemon: DaemonConfig{
			PollDefaultInterval: 60,
			WebhookListenAddr:   "127.0.0.1:2020",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Ports: PortsConfig{
			AutoAssignRangeStart: 40000,
			AutoAssignRangeEnd:   49999,
		},
	}
}

// Load reads a TOML config file from path. Missing file returns Default().
// Malformed file returns an error.
func Load(path string) (*Config, error) {
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Write serialises cfg to path in TOML format. The directory must already exist.
func Write(cfg *Config, path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
