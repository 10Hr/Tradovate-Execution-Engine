package tradovate

import (
	"encoding/json"
	"sync"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"

	"github.com/gorilla/websocket"
)

// DataSubscriber manages real-time market data subscriptions
type DataSubscriber struct {
	client        marketdata.WebSocketSender
	subscriptions map[string]string
	mu            sync.RWMutex
	log           *logger.Logger

	// Custom handlers
	OnQuoteUpdate    func(quote marketdata.Quote)
	OnChartUpdate    func(chart marketdata.ChartUpdate)
	OnOrderUpdate    func(data json.RawMessage)
	OnPositionUpdate func(data json.RawMessage)
	OnUserSync       func(data json.RawMessage)
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
