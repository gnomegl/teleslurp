package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey      string `json:"api_key"`
	TGAPIID     int    `json:"tg_api_id,omitempty"`
	TGAPIHash   string `json:"tg_api_hash,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

type MonitorSource struct {
	ID       int64  `yaml:"id,omitempty"`
	Username string `yaml:"username,omitempty"`
}

type MonitorTarget struct {
	ID       int64  `yaml:"id,omitempty"`
	Username string `yaml:"username,omitempty"`
}

type MonitorConfig struct {
	SourceChannels []MonitorSource `yaml:"source_channels"`
	SourceGroups   []MonitorSource `yaml:"source_groups"`
	TargetChannels []MonitorTarget `yaml:"target_channels"`
	MonitorUsers   []MonitorSource `yaml:"monitor_users,omitempty"`
}

func GetConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(homeDir, "AppData", "Local", "teleslurp")
	}
	return filepath.Join(homeDir, ".config", "teleslurp")
}

func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

func GetSessionPath() string {
	return filepath.Join(GetConfigDir(), "session.json")
}

func GetDatabasePath() string {
	return filepath.Join(GetConfigDir(), "teleslurp.db")
}

func GetMonitorConfigPath() string {
	return filepath.Join(GetConfigDir(), "monitor.config.yaml")
}

func Load() (*Config, error) {
	configPath := GetConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func Save(config *Config) error {
	configPath := GetConfigPath()
	configDir := filepath.Dir(configPath)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func LoadMonitorConfig() (*MonitorConfig, error) {
	configPath := GetMonitorConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("monitor config file not found: %s", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading monitor config: %w", err)
	}

	var config MonitorConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing monitor config: %w", err)
	}

	return &config, nil
}

func SaveMonitorConfig(config *MonitorConfig) error {
	configPath := GetMonitorConfigPath()
	configDir := filepath.Dir(configPath)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("error marshaling monitor config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}
