package auth

import (
	"sync"
	"time"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"
)

//
// TOKEN MANAGER
//

// TokenManager manages authentication tokens for Tradovate API
type TokenManager struct {
	mu              sync.RWMutex
	accessToken     string
	mdAccessToken   string
	expirationTime  time.Time
	userID          int
	accountID       int
	username        string
	credentials     map[string]interface{}
	baseURL         string
	log             *logger.Logger
	config          config.Config
	monitorStopChan chan struct{}
}
