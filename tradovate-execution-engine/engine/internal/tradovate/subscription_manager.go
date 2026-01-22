package tradovate

import (
	"encoding/json"
	"fmt"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
	//"tradovate-execution-engine/engine/internal/marketdata"
)

// NewDataSubscriber creates a new market data subscriber
func NewDataSubscriber(client marketdata.WebSocketSender) *DataSubscriber {
	return &DataSubscriber{
		client:        client,
		subscriptions: make(map[string]string),
	}
}

// SetLogger sets the logger for the subscriber
func (s *DataSubscriber) SetLogger(l *logger.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = l
}

// IsConnected returns whether the underlying client is connected
func (s *DataSubscriber) IsConnected() bool {
	return s.client.IsConnected()
}

// Connect attempts to connect the underlying client
func (s *DataSubscriber) Connect() error {
	return s.client.Connect()
}

// HandleEvent processes incoming market data events
func (s *DataSubscriber) HandleEvent(eventType string, data json.RawMessage) {
	switch eventType {
	case marketdata.EventMarketData:
		s.handleMarketData(data)
	case marketdata.EventChart, "md/getchart":
		s.handleChartData(data)
	case marketdata.EventUser:
		if s.log != nil {
			/*s.log.Infof*/ fmt.Printf("Event Received: %s\n", eventType)
		}
		// User sync often contains positions and orders
		if s.OnUserSync != nil {
			s.OnUserSync(data)
		}
		s.handleUserEvent(data)
	case marketdata.EventOrder:
		if s.OnOrderUpdate != nil {
			s.OnOrderUpdate(data)
		}
	case marketdata.EventPosition:
		if s.OnPositionUpdate != nil {
			s.OnPositionUpdate(data)
		}
	case marketdata.EventProps:
		s.handlePropsEvent(data)
	case "md/subscribequote", "md/unsubscribequote", "md/subscribechart", "md/unsubscribechart":
		// These are response confirmations for requests, ignore them to avoid "Unknown event type" warnings
		if s.log != nil {
			/*s.log.Debugf*/ fmt.Printf("\nConfirmation received for: %s", eventType)
		}
	default:
		if s.log != nil {
			/*s.log.Warnf*/ fmt.Printf("Unknown event type: %s", eventType)
		}
	}
}

// AddQuoteHandler adds a callback for quote updates
func (s *DataSubscriber) AddQuoteHandler(handler func(marketdata.Quote)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OnQuoteUpdate = append(s.OnQuoteUpdate, handler)
}

// AddChartHandler adds a callback for chart updates
func (s *DataSubscriber) AddChartHandler(handler func(marketdata.ChartUpdate)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OnChartUpdate = append(s.OnChartUpdate, handler)
}

// handlePropsEvent handles incremental updates
func (s *DataSubscriber) handlePropsEvent(data json.RawMessage) {
	var props struct {
		EntityType string          `json:"entityType"`
		Entity     json.RawMessage `json:"entity"`
	}
	if err := json.Unmarshal(data, &props); err != nil {
		if s.log != nil {
			/*s.log.Warnf*/ fmt.Printf("Failed to parse props event: %v", err)
		}
		return
	}

	switch props.EntityType {
	case "order":
		if s.OnOrderUpdate != nil {
			s.OnOrderUpdate(props.Entity)
		}
	case "position":
		if s.OnPositionUpdate != nil {
			s.OnPositionUpdate(props.Entity)
		}
	}
}

// handleUserEvent processes user sync events which may contain orders/positions
func (s *DataSubscriber) handleUserEvent(data json.RawMessage) {
	// UserSync response structure
	var syncData struct {
		Orders    []json.RawMessage `json:"orders"`
		Positions []json.RawMessage `json:"positions"`
	}

	if err := json.Unmarshal(data, &syncData); err == nil {
		// If successfully parsed as a sync object, iterate and dispatch
		if s.OnOrderUpdate != nil {
			for _, order := range syncData.Orders {
				s.OnOrderUpdate(order)
			}
		}
		if s.OnPositionUpdate != nil {
			for _, pos := range syncData.Positions {
				s.OnPositionUpdate(pos)
			}
		}
		return
	}

	// Fallback: If it's not the standard sync object, pass generic data
	// (Though user/syncrequest usually follows that structure)
	if s.OnOrderUpdate != nil {
		s.OnOrderUpdate(data)
	}
	if s.OnPositionUpdate != nil {
		s.OnPositionUpdate(data)
	}
}

// handleMarketData processes market data events (quotes)
func (s *DataSubscriber) handleMarketData(data json.RawMessage) {
	quoteData, err := marketdata.ParseQuoteData(data)
	if err != nil {
		if s.log != nil {
			/*s.log.Errorf*/ fmt.Printf("Error unmarshaling quote data: %v", err)
		}
		return
	}

	s.mu.RLock()
	handlers := s.OnQuoteUpdate
	s.mu.RUnlock()

	for _, quote := range quoteData.Quotes {
		// Call custom handlers if set
		for _, handler := range handlers {
			handler(quote)
		}
	}
}

// handleChartData processes chart/tick data events
func (s *DataSubscriber) handleChartData(data json.RawMessage) {
	chartUpdate, err := marketdata.ParseChartData(data)
	if err != nil {
		// Try to parse as single chart (sometimes md/getchart returns this in some cases)
		var singleChart marketdata.Chart
		if err2 := json.Unmarshal(data, &singleChart); err2 == nil {
			chartUpdate = &marketdata.ChartUpdate{Charts: []marketdata.Chart{singleChart}}
		} else {
			if s.log != nil {
				/*s.log.Errorf*/ fmt.Printf("Error unmarshaling chart data: %v, payload: %s", err, string(data))
			}
			return
		}
	}

	for _, chart := range chartUpdate.Charts {
		if len(chart.Ticks) > 0 {
			if s.log != nil {
				/*s.log.Debugf*/ fmt.Printf("Tick data received - Chart ID: %d, Ticks: %d", chart.ID, len(chart.Ticks))
			}
		}

		if len(chart.Bars) > 0 {
			if s.log != nil {
				/*s.log.Debugf*/ fmt.Printf("Bar data received - Chart ID: %d, Bars: %d", chart.ID, len(chart.Bars))
			}
		}
	}

	s.mu.RLock()
	handlers := s.OnChartUpdate
	s.mu.RUnlock()

	// Call custom handlers if set
	for _, handler := range handlers {
		handler(*chartUpdate)
	}
}

// SubscribeQuote subscribes to real-time quote data for a symbol
func (s *DataSubscriber) SubscribeQuote(symbol interface{}) error {
	s.mu.Lock()
	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeQuote, symbol)
	if _, exists := s.subscriptions[key]; exists {
		s.mu.Unlock()
		return nil // Already subscribed
	}
	s.mu.Unlock()

	body := map[string]interface{}{
		"symbol": symbol,
	}

	if err := s.client.Send("md/subscribequote", body); err != nil {
		return err
	}

	s.mu.Lock()
	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
	s.mu.Unlock()

	if s.log != nil {
		/*s.log.Infof*/ fmt.Printf("Subscribed to quotes for %v", symbol)
	}
	return nil
}

// UnsubscribeQuote unsubscribes from quote data
func (s *DataSubscriber) UnsubscribeQuote(symbol interface{}) error {
	body := map[string]interface{}{
		"symbol": symbol,
	}

	if err := s.client.Send("md/unsubscribequote", body); err != nil {
		return err
	}

	s.mu.Lock()
	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeQuote, symbol)
	delete(s.subscriptions, key)
	s.mu.Unlock()

	if s.log != nil {
		/*s.log.Infof*/ fmt.Printf("Unsubscribed from quotes for %v", symbol)
	}
	return nil
}

// SubscribeTickChart subscribes to real-time tick data
func (s *DataSubscriber) SubscribeTickChart(symbol interface{}) error {
	params := map[string]interface{}{
		"symbol": symbol,
		"chartDescription": map[string]interface{}{
			"underlyingType": "Tick",
		},
	}

	if err := s.client.Send("md/subscribechart", params); err != nil {
		return err
	}

	s.mu.Lock()
	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeTick, symbol)
	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
	s.mu.Unlock()

	if s.log != nil {
		/*s.log.Infof*/ fmt.Printf("Subscribed to tick chart for %v", symbol)
	}
	return nil
}

// SubscribeMinuteChart subscribes to real-time minute bar data
func (s *DataSubscriber) SubscribeMinuteChart(symbol interface{}) error {
	params := map[string]interface{}{
		"symbol": symbol,
		"chartDescription": map[string]interface{}{
			"underlyingType":  "MinuteBar",
			"elementSize":     1,
			"elementSizeUnit": "UnderlyingUnits",
		},
	}

	if err := s.client.Send("md/subscribechart", params); err != nil {
		fmt.Print(err)
		return err
	}

	s.mu.Lock()
	key := fmt.Sprintf("%s: %v", "Minute Bars ", symbol)
	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
	s.mu.Unlock()

	if s.log != nil {
		fmt.Printf("Subscribed to minute chart for %v\n", symbol)
	}
	return nil
}

// UnsubscribeChart unsubscribes from chart data
func (s *DataSubscriber) UnsubscribeChart(symbol interface{}) error {
	body := map[string]interface{}{
		"symbol": symbol,
	}

	if err := s.client.Send("md/unsubscribechart", body); err != nil {
		return err
	}

	s.mu.Lock()
	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeTick, symbol)
	delete(s.subscriptions, key)
	s.mu.Unlock()

	if s.log != nil {
		/*s.log.Infof*/ fmt.Printf("Unsubscribed from tick chart for %v", symbol)
	}
	return nil
}

// GetChart requests chart data (historical and/or live)
// This implements the md/getchart endpoint
func (s *DataSubscriber) GetChart(params marketdata.HistoricalDataParams) error {
	if err := s.client.Send("md/getchart", params); err != nil {
		return err
	}

	if s.log != nil {
		/*s.log.Infof*/ fmt.Printf("Requested chart data for %v", params.Symbol)
	}
	return nil
}

func (s *DataSubscriber) SubscribeUserSyncRequests(users []int) error {
	body := map[string]interface{}{
		"users": users,
	}

	if err := s.client.Send("user/syncrequest", body); err != nil {
		return err
	}

	if s.log != nil {
		/*s.log.Info*/ fmt.Println("Subscribed to user sync requests")
	}
	return nil
}

// UnsubscribeAll unsubscribes from all active subscriptions
func (s *DataSubscriber) UnsubscribeAll() error {
	s.mu.Lock()
	subscriptions := make(map[string]string)
	for k, v := range s.subscriptions {
		subscriptions[k] = v
	}
	s.mu.Unlock()

	for key, symbol := range subscriptions {
		if len(key) >= 5 {
			subscriptionType := key[:5]
			switch subscriptionType {
			case marketdata.SubscriptionTypeQuote:
				if err := s.UnsubscribeQuote(symbol); err != nil {
					if s.log != nil {
						/*s.log.Errorf*/ fmt.Printf("Error unsubscribing quote for %v: %v", symbol, err)
					}
				}
			case marketdata.SubscriptionTypeTick + ":":
				if err := s.UnsubscribeChart(symbol); err != nil {
					if s.log != nil {
						/*s.log.Errorf*/ fmt.Printf("Error unsubscribing chart for %v: %v", symbol, err)
					}
				}
			}
		}
	}

	return nil
}

// GetActiveSubscriptions returns a copy of active subscriptions
func (s *DataSubscriber) GetActiveSubscriptions() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subscriptions := make(map[string]string)
	for k, v := range s.subscriptions {
		subscriptions[k] = v
	}
	return subscriptions
}
