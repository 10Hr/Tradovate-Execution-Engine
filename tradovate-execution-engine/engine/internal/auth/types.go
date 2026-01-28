package auth

import (
	"sync"
	"time"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"
)

// AuthResponse represents the Tradovate authentication response
type AuthResponse struct {
	AccessToken    string    `json:"accessToken"`
	ExpirationTime time.Time `json:"expirationTime"`
	MDAccessToken  string    `json:"mdAccessToken"`
	UserID         int       `json:"userId"`
	Name           string    `json:"name"`
	OrgName        string    `json:"orgName"`
	UserStatus     string    `json:"userStatus"`
	HasMarketData  bool      `json:"hasMarketData"`
	HasFunded      bool      `json:"hasFunded"`
	HasLive        bool      `json:"hasLive"`
}

// Account represents a Tradovate account
type Account struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	AccountType string `json:"accountType"`
	Active      bool   `json:"active"`
}

// TokenManager manages authentication tokens for Tradovate API
type TokenManager struct {
	mu             sync.RWMutex
	accessToken    string
	mdAccessToken  string
	expirationTime time.Time
	userID         int
	accountID      int
	username       string
	credentials    map[string]interface{}
	baseURL        string
	log            *logger.Logger
	config         config.Config
	monitorStopChan chan struct{}
}
