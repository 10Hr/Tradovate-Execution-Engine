package marketdata

import "encoding/json"

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
	Ticks     []Tick `json:"ticks,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type Bar struct {
	Timestamp   string  `json:"timestamp"`
	Open        float64 `json:"open"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	Close       float64 `json:"close"`
	UpVolume    float64 `json:"upVolume"`
	DownVolume  float64 `json:"downVolume"`
	UpTicks     int     `json:"upTicks"`
	DownTicks   int     `json:"downTicks"`
	BidVolume   float64 `json:"bidVolume"`
	OfferVolume float64 `json:"offerVolume"`
}

type Tick struct {
	Timestamp string  `json:"timestamp"`
	Price     float64 `json:"price"`
	Size      float64 `json:"size"`
	BidPrice  float64 `json:"bidPrice,omitempty"`
	AskPrice  float64 `json:"askPrice,omitempty"`
	BidSize   float64 `json:"bidSize,omitempty"`
	AskSize   float64 `json:"askSize,omitempty"`
}

// Historical data request parameters
type HistoricalDataParams struct {
	Symbol           interface{} `json:"symbol"`
	ChartDescription ChartDesc   `json:"chartDescription"`
	TimeRange        TimeRange   `json:"timeRange"`
}

type ChartDesc struct {
	UnderlyingType  string `json:"underlyingType"` // "Tick", "DailyBar", "MinuteBar", etc.
	ElementSize     int    `json:"elementSize,omitempty"`
	ElementSizeUnit string `json:"elementSizeUnit,omitempty"` // "UnderlyingUnits", "Volume", etc.
	WithHistogram   bool   `json:"withHistogram,omitempty"`
}

type TimeRange struct {
	ClosestTimestamp string `json:"closestTimestamp,omitempty"` // ISO 8601 format
	ClosestTickID    int    `json:"closestTickId,omitempty"`
	AsFarAsTimestamp string `json:"asFarAsTimestamp,omitempty"` // ISO 8601 format
}

// Event types
const (
	EventMarketData = "md"
	EventChart      = "chart"
)

// Subscription types
const (
	SubscriptionTypeQuote = "quote"
	SubscriptionTypeTick  = "tick"
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
