package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds all configuration settings
type Config struct {
	Tradovate TradovateConfig `json:"tradovate"`
}

// TradovateConfig holds Tradovate-specific credentials
type TradovateConfig struct {
	AppID       string `json:"appId"`
	AppVersion  string `json:"appVersion"`
	Chl         string `json:"chl"`
	Cid         string `json:"cid"`
	DeviceID    string `json:"deviceId"`
	Environment string `json:"environment"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Sec         string `json:"sec"`
	Enc         bool   `json:"enc"`
}

// LoadConfig reads the configuration from a JSON file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// SaveConfig writes a default configuration to a JSON file
func SaveConfig(filename string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// CreateDefaultConfig creates a template configuration file
func CreateDefaultConfig(filename string) error {
	defaultConfig := &Config{
		Tradovate: TradovateConfig{
			AppID:       "tradovate_trader(web)",
			AppVersion:  "3.260109.0",
			Chl:         "I genuinely have no idea what this is",
			Cid:         "1",
			DeviceID:    "<your_device_id_here>",
			Environment: "demo",
			Username:    "your_username_here",
			Password:    "your_password_here",
			Sec:         "your_security_token_here",
			Enc:         true,
		},
	}

	return SaveConfig(filename, defaultConfig)
}
