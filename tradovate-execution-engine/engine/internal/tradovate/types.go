package tradovate

import (
	"encoding/json"
	"sync"
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

// APIPosition represents the Tradovate position response
type APIPosition struct {
	ID          int     `json:"id"`
	ContractID  int     `json:"contractId"`
	NetPos      int     `json:"netPos"`
	NetPrice    float64 `json:"netPrice"`
	BoughtValue float64 `json:"boughtValue"`
	SoldValue   float64 `json:"soldValue"`
}

// APIOrderEvent represents the Tradovate order event
type APIOrderEvent struct {
	ID           int     `json:"id"`
	OrderID      int     `json:"orderId"`
	ContractID   int     `json:"contractId"`
	Action       string  `json:"action"`    // Buy, Sell
	OrdStatus    string  `json:"ordStatus"` // Pending, Working, Filled, Cancelled, Rejected
	FilledQty    int     `json:"cumQty"`    // Cumulative filled quantity
	AvgFillPrice float64 `json:"avgPrice"`
	RejectReason string  `json:"rejectReason"`
	Text         string  `json:"text"`
}
