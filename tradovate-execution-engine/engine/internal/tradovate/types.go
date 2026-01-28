package tradovate

import (
	"encoding/json"
	"sync"
	"time"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"

	"github.com/gorilla/websocket"
)

// SubscriptionInfo tracks details about an active subscription
type SubscriptionInfo struct {
	Endpoint string                 // e.g., "md/subscribequote"
	Params   map[string]interface{} // The body/params that uniquely identify this subscription
	ChartID  int                    // For chart subscriptions (returned by server)
	RefCount int                    // Reference counting for shared subscriptions
}

type DataSubscriber struct {
	client        marketdata.WebSocketSender
	log           *logger.Logger
	mu            sync.RWMutex
	subscriptions map[string]*SubscriptionInfo // key: hash of endpoint+params

	// Callbacks
	OnQuoteUpdate       []func(marketdata.Quote)
	OnChartUpdate       []func(marketdata.ChartUpdate)
	OnOrderUpdate       func(json.RawMessage)
	OnPositionUpdate    func(json.RawMessage)
	OnUserSync          func(json.RawMessage)
	OnCashBalanceUpdate func(json.RawMessage)
}

// MessageHandler is a callback for processing incoming WebSocket messages
type MessageHandler func(eventType string, data json.RawMessage)

// TradovateWebSocketClient manages WebSocket connection lifecycle
type TradovateWebSocketClient struct {
	accessToken  string
	wsURL        string
	conn         *websocket.Conn
	isAuthorized bool
	mu           sync.RWMutex
	log          *logger.Logger

	// Message handler for routing events
	messageHandler MessageHandler

	// Refinements
	nextRequestID   uint32
	openChan        chan struct{}
	pendingRequests map[uint32]string
	heartbeatStop   chan struct{}
}

// WSResponse represents a WebSocket response from Tradovate
type WSResponse struct {
	ID         int             `json:"i,omitempty"`
	Status     int             `json:"s,omitempty"`
	Event      string          `json:"e,omitempty"`
	Data       json.RawMessage `json:"d,omitempty"`
	StatusText string          `json:"statusText,omitempty"`
}

// APIPosition represents a Tradovate position
type APIPosition struct {
	ContractID int     `json:"contractId"`
	NetPos     int     `json:"netPos"`
	Bought     int     `json:"bought"`
	NetPrice   float64 `json:"netPrice"`
	PrevPrice  float64 `json:"prevPrice"`
}

// APIContract represents a Tradovate contract
type APIContract struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type APITradeDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type APICashBalance struct {
	RealizedPnL float64 `json:"realizedPnL"`
}

// APIProduct represents a Tradovate product
type APIProduct struct {
	Name          string  `json:"name"`
	ValuePerPoint float64 `json:"valuePerPoint"`
}

// APIUserSyncData represents the initial user sync response
type APIUserSyncData struct {
	Users []struct {
		ID int `json:"id"`
	} `json:"users,omitempty"`
	Positions    []APIPosition     `json:"positions,omitempty"`
	Contracts    []APIContract     `json:"contracts,omitempty"`
	Products     []APIProduct      `json:"products,omitempty"`
	CashBalances []json.RawMessage `json:"cashBalances"`
	Orders       []json.RawMessage `json:"orders"`
}

// APIAuthResponse represents the Tradovate authentication response
type APIAuthResponse struct {
	AccessToken    string    `json:"accessToken"`
	ExpirationTime time.Time `json:"expirationTime"`
	MDAccessToken  string    `json:"mdAccessToken"`
	UserID         int       `json:"userId"`
	Name           string    `json:"name"`
}

// APIAccount represents a Tradovate account
type APIAccount struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
