package models

import (
	"time"
)

// OrderStatus represents the current status of an order
type OrderStatus string

const (
	StatusPending   OrderStatus = "PENDING"
	StatusSubmitted OrderStatus = "SUBMITTED"
	StatusFilled    OrderStatus = "FILLED"
	StatusRejected  OrderStatus = "REJECTED"
	StatusCanceled  OrderStatus = "CANCELED"
	StatusFailed    OrderStatus = "FAILED"
)

// OrderSide represents buy or sell
type OrderSide string

const (
	SideBuy  OrderSide = "Buy"
	SideSell OrderSide = "Sell"
)

// OrderType represents the type of order
type OrderType string

const (
	TypeMarket OrderType = "Market"
	TypeLimit  OrderType = "Limit"
	TypeStop   OrderType = "Stop"
)

// Order represents a trading order
type Order struct {
	ID           string      // Unique order identifier
	Symbol       string      // Trading symbol
	Side         OrderSide   // Buy or Sell
	Type         OrderType   // Market, Limit, Stop
	Quantity     int         // Number of contracts
	Price        float64     // Order price (0 for market orders)
	Status       OrderStatus // Current order status
	SubmittedAt  time.Time   // When order was submitted
	RejectReason string      // Reason for rejection if applicable
	ExternalID   string      // External order ID from broker
}
