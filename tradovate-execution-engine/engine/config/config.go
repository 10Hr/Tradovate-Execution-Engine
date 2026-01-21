package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GetHTTPBaseURL returns the HTTP API base URL for the given environment
func GetHTTPBaseURL(environment string) string {
	if environment == "live" {
		return "https://live.tradovateapi.com"
	}
	return "https://demo.tradovateapi.com"
}

// GetWSBaseURL returns the Market Data WebSocket API base URL for the given environment
func GetMDWSBaseURL(environment string) string {

	if environment == "live" {
		return "wss://md-live.tradovateapi.com/v1/websocket"
	}
	return "wss://md-demo.tradovateapi.com/v1/websocket"
}

// GetWSBaseURL returns the WebSocket API base URL for the given environment
func GetWSBaseURL(environment string) string {

	if environment == "live" {
		return "wss://live.tradovateapi.com/v1/websocket"
	}
	return "wss://demo.tradovateapi.com/v1/websocket"
}

// GetConfigPath returns the absolute path to the config file
func GetConfigPath() string {
	rootDir := GetProjectRoot()
	return filepath.Join(rootDir, "config", "config.json")
}

// LoadOrCreateConfig loads the config file, creating a default one if it doesn't exist
func LoadOrCreateConfig() (*Config, error) {
	configPath := GetConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config doesn't exist, create default
		fmt.Printf("Config file not found at %s. Creating default config...\n", configPath)
		if err := CreateDefaultConfig(configPath); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		fmt.Printf("Default config created at %s\n", configPath)
		fmt.Println("Please edit the config file with your credentials and restart.")
		return nil, fmt.Errorf("default config created, please configure and restart")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	fmt.Printf("Config loaded successfully from %s\n", configPath)
	return &config, nil
}

// GetProjectRoot searches for go.mod to identify the project root and returns its absolute path
func GetProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	// Search up to 5 levels for go.mod
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return dir
			}
			return absDir
		}

		parent := filepath.Dir(dir)
		if parent == dir { // Reached system root
			break
		}
		dir = parent
	}

	return "."
}

// SaveConfig writes a default configuration to a JSON file
func SaveConfig(filename string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(filename, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// CreateDefaultConfig creates a template configuration file
func CreateDefaultConfig(path string) error {
	defaultConfig := &Config{
		Tradovate: TradovateConfig{
			AppID:       "your_app_id_here",
			AppVersion:  "your_app_version_here",
			Chl:         "your_chl_here",
			Cid:         "your_cid_here",
			DeviceID:    "your_device_id_here",
			Environment: "demo",
			Username:    "your_username_here",
			Password:    "your_password_here",
			Sec:         "your_security_token_here",
			Enc:         true,
		},
		Risk: RiskConfig{
			MaxContracts:     1,
			DailyLossLimit:   500.0,
			MaxOrderRetries:  3,
			EnableRiskChecks: true,
		},
	}

	return SaveConfig(path, defaultConfig)
}

// CreateDefaultConfig overload
func CreateConfigFromParams(appId, appVersion, chl, cid, deviceId, env, user, pass, sec string, enc bool, maxContracts, maxOrderRetries int, dailyLossLimit float64, enableRiskCheck bool) error {
	defaultConfig := &Config{
		Tradovate: TradovateConfig{
			AppID:       appId,
			AppVersion:  appVersion,
			Chl:         chl,
			Cid:         cid,
			DeviceID:    deviceId,
			Environment: env,
			Username:    user,
			Password:    pass,
			Sec:         sec,
			Enc:         enc,
		},
		Risk: RiskConfig{
			MaxContracts:     maxContracts,
			DailyLossLimit:   dailyLossLimit,
			MaxOrderRetries:  maxOrderRetries,
			EnableRiskChecks: enableRiskCheck,
		},
	}

	return SaveConfig("config", defaultConfig)
}
