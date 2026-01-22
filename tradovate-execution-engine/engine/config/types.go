package config

// Config holds all configuration settings
type Config struct {
	Tradovate TradovateConfig `json:"tradovate"`
	Risk      RiskConfig      `json:"risk"`
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

// RiskConfig holds risk management and order configuration
type RiskConfig struct {
	MaxContracts     int     `json:"maxContracts"`
	DailyLossLimit   float64 `json:"dailyLossLimit"`
	EnableRiskChecks bool    `json:"enableRiskChecks"`
}
