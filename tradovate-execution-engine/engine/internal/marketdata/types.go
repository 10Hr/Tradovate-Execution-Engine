package marketdata

import "encoding/json"

//
// MARKETDATA
//

// WebSocketSender is an interface for sending messages through WebSocket
type WebSocketSender interface {
	Send(url string, body interface{}) error
	IsConnected() bool
	Connect() error
}

// Market data structures
type Quote struct {
	Timestamp  string           `json:"timestamp"`
	ContractID int              `json:"contractId"`
	Entries    map[string]Entry `json:"entries"`
}

type Entry struct {
	Price float64 `json:"price,omitempty"`
	Size  float64 `json:"size,omitempty"`
}

type QuoteData struct {
	Quotes []Quote `json:"quotes"`
}

// Chart/Tick data structures
type ChartUpdate struct {
	Charts []Chart `json:"charts"`
}

type Chart struct {
	ID        int    `json:"id"`
	Bars      []Bar  `json:"bars,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	EOH       bool   `json:"eoh,omitempty"`
}

type Bar struct {
	Timestamp string  `json:"timestamp"`
	Close     float64 `json:"close"`
}

// Historical data request parameters
type HistoricalDataParams struct {
	Symbol           interface{} `json:"symbol"`
	ChartDescription ChartDesc   `json:"chartDescription"`
	TimeRange        TimeRange   `json:"timeRange"`
}

type ChartDesc struct {
	UnderlyingType  string `json:"underlyingType"`            // "Tick", "DailyBar", "MinuteBar", etc.
	ElementSize     int    `json:"elementSize,omitempty"`     // Type intervals ex: "1" with UnderlyingType being "MinuteBar" = 1 Minute Bars
	ElementSizeUnit string `json:"elementSizeUnit,omitempty"` // "UnderlyingUnits", "Volume", etc.
}

type TimeRange struct {
	ClosestTimestamp string `json:"closestTimestamp,omitempty"`
	AsMuchAsElements int    `json:"asMuchAsElements,omitempty"`
}

// Event types
const (
	EventMarketData  = "md"
	EventChart       = "chart"
	EventUser        = "user/syncrequest"
	EventOrder       = "order"
	EventPosition    = "position"
	EventCashBalance = "cashBalance"
	EventProps       = "props"
)

// Helper function to parse quote data from raw JSON
func ParseQuoteData(data json.RawMessage) (*QuoteData, error) {
	var quoteData QuoteData
	if err := json.Unmarshal(data, &quoteData); err != nil {
		return nil, err
	}
	return &quoteData, nil
}

// Helper function to parse chart data from raw JSON
func ParseChartData(data json.RawMessage) (*ChartUpdate, error) {
	var chartUpdate ChartUpdate
	if err := json.Unmarshal(data, &chartUpdate); err != nil {
		return nil, err
	}
	return &chartUpdate, nil
}
