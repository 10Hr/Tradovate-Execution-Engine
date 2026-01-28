package portfolio

import (
	"encoding/json"
	"sync"
	"time"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
	"tradovate-execution-engine/engine/internal/tradovate"
)

// Position represents a Tradovate position
type Position struct {
	ID          int       `json:"id"`
	AccountID   int       `json:"accountId"`
	ContractID  int       `json:"contractId"`
	Timestamp   time.Time `json:"timestamp"`
	TradeDate   TradeDate `json:"tradeDate"`
	NetPos      int       `json:"netPos"`
	Bought      int       `json:"bought"`
	BoughtValue float64   `json:"boughtValue"`
	Sold        int       `json:"sold"`
	SoldValue   float64   `json:"soldValue"`
	PrevPos     int       `json:"prevPos"`
	Archived    bool      `json:"archived"`
	NetPrice    float64   `json:"netPrice"`
	PrevPrice   float64   `json:"prevPrice"`
}

// Contract represents a Tradovate contract
type Contract struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type TradeDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type CashBalance struct {
	ID              int       `json:"id"`
	AccountID       int       `json:"accountId"`
	Timestamp       time.Time `json:"timestamp"`
	TradeDate       TradeDate `json:"tradeDate"`
	CurrencyID      int       `json:"currencyId"`
	Amount          float64   `json:"amount"`
	RealizedPnL     float64   `json:"realizedPnL"`
	WeekRealizedPnL float64   `json:"weekRealizedPnL"`
	AmountSOD       float64   `json:"amountSOD"`
	Archived        bool      `json:"archived"`
}

// Product represents a Tradovate product
type Product struct {
	Name          string  `json:"name"`
	ValuePerPoint float64 `json:"valuePerPoint"`
}

type FillPair struct {
	ID         int     `json:"id"`
	PositionID int     `json:"positionId"`
	BuyFillID  int     `json:"buyFillId"`
	SellFillID int     `json:"sellFillId"`
	Qty        int     `json:"qty"`
	BuyPrice   float64 `json:"buyPrice"`
	SellPrice  float64 `json:"sellPrice"`
	Active     bool    `json:"active"`
	Archived   bool    `json:"archived"`
}

// UserSyncData represents the initial user sync response
type UserSyncData struct {
	Users []struct {
		ID int `json:"id"`
	} `json:"users,omitempty"`
	Positions    []Position        `json:"positions,omitempty"`
	Contracts    []Contract        `json:"contracts,omitempty"`
	Products     []Product         `json:"products,omitempty"`
	CashBalances []json.RawMessage `json:"cashBalances"`
	Orders       []json.RawMessage `json:"orders"`
	FillPairs    []json.RawMessage `json:"fillPairs"`
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

	realizedPnL float64 // Today's closed trade P&L
}

// PortfolioTracker manages the entire portfolio tracking system
type PortfolioTracker struct {
	tradingSubsciptionManager *tradovate.DataSubscriber
	mdSubsciptionManager      *tradovate.DataSubscriber
	plTracker                 *PLTracker
	log                       *logger.Logger
	running                   bool
	mu                        sync.Mutex

	// State tracking
	userID    int
	positions map[int]*Position
	contracts map[int]string
	products  map[string]float64
}
