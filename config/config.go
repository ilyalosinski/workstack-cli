package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	BaseDir  string `json:"base_dir"`
	Profiles []struct {
		Name    string `json:"name"`
		Command string `json:"command"`
	} `json:"profiles"`
}

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		BaseDir: filepath.Join(home, "Development"),
		Profiles: []struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		}{
			{Name: "claude", Command: "claude --dangerously-skip-permissions"},
			{Name: "codex", Command: "codex --full-auto"},
		},
	}
}

func Load() *Config {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "workstack", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig()
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}
	return &cfg
}
