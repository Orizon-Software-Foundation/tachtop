// Package config loads tachtop's optional JSON configuration file.
//
// tachtop works perfectly with no configuration at all. If a config file is
// present it can change the default theme, refresh rate and temperature unit.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultRefreshMS is the refresh interval used when nothing else is set.
const DefaultRefreshMS = 1500

// MinRefreshMS and MaxRefreshMS keep a bad config from freezing the UI or
// hammering /proc.
const (
	MinRefreshMS = 250
	MaxRefreshMS = 60000
)

// Config mirrors config.json. Every field is optional.
type Config struct {
	Theme     string `json:"theme"`
	RefreshMS int    `json:"refresh_ms"`
	TempUnit  string `json:"temp_unit"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Theme:     "tachtop",
		RefreshMS: DefaultRefreshMS,
		TempUnit:  "c",
	}
}

// Path returns the default config location, honouring XDG_CONFIG_HOME.
// It returns "" when no home directory can be determined.
func Path() string {
	if p := os.Getenv("TACHTOP_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tachtop", "config.json")
}

// Load reads the config file at path. A missing file is not an error: the
// defaults are returned. An unreadable or malformed file returns an error
// alongside the defaults so callers can warn and continue.
func Load(path string) (Config, string, error) {
	cfg := Default()
	if path == "" {
		path = Path()
	}
	if path == "" {
		return cfg, "", nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, path, nil
	}
	if err != nil {
		return cfg, path, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), path, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.normalize()
	return cfg, path, nil
}

func (c *Config) normalize() {
	if c.Theme == "" {
		c.Theme = "tachtop"
	}
	if c.RefreshMS == 0 {
		c.RefreshMS = DefaultRefreshMS
	}
	if c.RefreshMS < MinRefreshMS {
		c.RefreshMS = MinRefreshMS
	}
	if c.RefreshMS > MaxRefreshMS {
		c.RefreshMS = MaxRefreshMS
	}
	c.TempUnit = strings.ToLower(c.TempUnit)
	if c.TempUnit != "f" {
		c.TempUnit = "c"
	}
}
