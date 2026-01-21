package portfolio

import (
	"sync"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
)

// Position represents a Tradovate position
type Position struct {
	ID         int     `json:"id"`
	ContractID int     `json:"contractId"`
	NetPos     int     `json:"netPos"`
	PrevPos    int     `json:"prevPos"`
	NetPrice   float64 `json:"netPrice"`
	PrevPrice  float64 `json:"prevPrice"`
}

// Contract represents a Tradovate contract
type Contract struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Product represents a Tradovate product
type Product struct {
	Name          string  `json:"name"`
	ValuePerPoint float64 `json:"valuePerPoint"`
}

// UserSyncData represents the initial user sync response
type UserSyncData struct {
	Users []struct {
		ID int `json:"id"`
	} `json:"users,omitempty"`
	Positions []Position `json:"positions,omitempty"`
	Contracts []Contract `json:"contracts,omitempty"`
	Products  []Product  `json:"products,omitempty"`
}

// PositionPL tracks P&L for a specific position
type PositionPL struct {
	Name          string
	PL            float64 // Unrealized PL
	RealizedPL    float64
	NetPos        int
	AvgPrice      float64
	ValuePerPoint float64
}

// PositionManager manages real-time P&L tracking for positions
type PositionManager struct {
	client      marketdata.WebSocketSender
	mdClient    marketdata.WebSocketSender
	contractMap map[int]string
	productMap  map[string]float64
	log         *logger.Logger
	userID      int

	Mu  sync.RWMutex
	Pls map[string]*PositionPL
	// Callbacks
	OnPLUpdate      func(name string, pl float64, netPos int)
	OnTotalPLUpdate func(totalPL float64)
}

// UserSyncResponse is the initial sync response
type UserSyncResponse struct {
	Users []struct {
		ID int `json:"id"`
	} `json:"users,omitempty"`
	Positions []Position `json:"positions,omitempty"`
	Contracts []Contract `json:"contracts,omitempty"`
	Products  []Product  `json:"products,omitempty"`
}

// PLEntry tracks P&L for a specific position
type PLEntry struct {
	Name      string
	PL        float64
	NetPos    int
	BuyPrice  float64
	LastPrice float64
}

// PLTracker manages profit & loss tracking
type PLTracker struct {
	entries map[string]*PLEntry
	mu      sync.RWMutex
	log     *logger.Logger
}
