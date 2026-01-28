package tradovate

import (
	"encoding/json"
	"fmt"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
)

// NewDataSubscriber creates a new market data subscriber
func NewDataSubscriptionManager(client marketdata.WebSocketSender) *DataSubscriber {
	return &DataSubscriber{
		client:        client,
		subscriptions: make(map[string]*SubscriptionInfo),
		OnQuoteUpdate: make([]func(marketdata.Quote), 0),
		OnChartUpdate: make([]func(marketdata.ChartUpdate), 0),
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

// makeSubscriptionKey creates a unique key for a subscription based on endpoint and params
func (s *DataSubscriber) makeSubscriptionKey(endpoint string, params map[string]interface{}) string {
	// Serialize params to JSON for consistent key
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		// Fallback to simple string concat if marshal fails
		return fmt.Sprintf("%s:%v", endpoint, params)
	}

	return fmt.Sprintf("%s:%s", endpoint, string(paramsJSON))
}

// isSubscribed checks if we already have an active subscription
func (s *DataSubscriber) isSubscribed(endpoint string, params map[string]interface{}) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.makeSubscriptionKey(endpoint, params)
	_, exists := s.subscriptions[key]
	return key, exists
}

// addSubscription adds or increments a subscription
func (s *DataSubscriber) addSubscription(endpoint string, params map[string]interface{}, chartID int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeSubscriptionKey(endpoint, params)

	if info, exists := s.subscriptions[key]; exists {
		info.RefCount++
		if chartID > 0 {
			info.ChartID = chartID
		}
		if s.log != nil {
			s.log.Infof("Incremented ref count for %s to %d", endpoint, info.RefCount)
		}
	} else {
		s.subscriptions[key] = &SubscriptionInfo{
			Endpoint: endpoint,
			Params:   params,
			ChartID:  chartID,
			RefCount: 1,
		}
		if s.log != nil {
			s.log.Infof("Added new subscription: %s with params %v", endpoint, params)
		}
	}

	return key
}

// removeSubscription decrements or removes a subscription
// Returns true if the subscription was completely removed, and the subscription info
func (s *DataSubscriber) removeSubscription(key string) (bool, *SubscriptionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.subscriptions[key]
	if !exists {
		return false, nil
	}

	info.RefCount--

	if info.RefCount <= 0 {
		// Make a copy before deleting
		infoCopy := &SubscriptionInfo{
			Endpoint: info.Endpoint,
			Params:   info.Params,
			ChartID:  info.ChartID,
			RefCount: info.RefCount,
		}
		delete(s.subscriptions, key)
		if s.log != nil {
			s.log.Infof("Removed subscription: %s", info.Endpoint)
		}
		return true, infoCopy
	}

	if s.log != nil {
		s.log.Infof("Decremented ref count for %s to %d", info.Endpoint, info.RefCount)
	}
	return false, info
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
			s.log.Infof("Event Received: %s", eventType)
		}
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
	case "md/subscribequote", "md/unsubscribequote":
		s.handleSubscriptionResponse(eventType)
	default:
		if s.log != nil {
			s.log.Infof("Unknown event type: %s", eventType)
		}
	}
}

// handleSubscriptionResponse processes subscription confirmations
func (s *DataSubscriber) handleSubscriptionResponse(eventType string) {
	if s.log != nil {
		s.log.Infof("Confirmation received for: %s", eventType)
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
			s.log.Errorf("Failed to parse props event: %v", err)
		}
		return
	}

	switch props.EntityType {
	case "order":
		if s.OnOrderUpdate != nil {
			s.OnOrderUpdate(props.Entity)
		}
	case marketdata.EventPosition:
		if s.OnPositionUpdate != nil {
			s.OnPositionUpdate(props.Entity)
		}
	case marketdata.EventCashBalance:
		if s.OnCashBalanceUpdate != nil {
			s.OnCashBalanceUpdate(props.Entity)
		}
	}
}

// handleUserEvent processes user sync events
func (s *DataSubscriber) handleUserEvent(data json.RawMessage) {

	var syncData APIUserSyncData

	if err := json.Unmarshal(data, &syncData); err == nil {
		if s.OnOrderUpdate != nil {
			for _, order := range syncData.Orders {
				s.OnOrderUpdate(order)
			}
		}
		if s.OnPositionUpdate != nil {
			for _, pos := range syncData.Positions {
				posJSON, _ := json.Marshal(pos)
				s.OnPositionUpdate(posJSON)
			}
		}
		if s.OnCashBalanceUpdate != nil {
			for _, bal := range syncData.CashBalances {
				s.OnCashBalanceUpdate(bal)
			}
		}

		return
	}

	// Fallback
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
			s.log.Errorf("Error unmarshaling quote data: %v", err)
		}
		return
	}

	s.mu.RLock()
	handlers := s.OnQuoteUpdate
	s.mu.RUnlock()

	for _, quote := range quoteData.Quotes {
		for _, handler := range handlers {
			handler(quote)
		}
	}
}

// handleChartData processes chart/tick data events
func (s *DataSubscriber) handleChartData(data json.RawMessage) {
	chartUpdate, err := marketdata.ParseChartData(data)
	if err != nil {
		var singleChart marketdata.Chart
		if err2 := json.Unmarshal(data, &singleChart); err2 == nil {
			chartUpdate = &marketdata.ChartUpdate{Charts: []marketdata.Chart{singleChart}}
		} else {
			if s.log != nil {
				s.log.Errorf("Error unmarshaling chart data: %v", err)
			}
			return
		}
	}

	for _, chart := range chartUpdate.Charts {
		if len(chart.Bars) > 0 && s.log != nil {
			s.log.Infof("Bar data received - Chart ID: %d, Bars: %d", chart.ID, len(chart.Bars))
		}
	}

	s.mu.RLock()
	handlers := s.OnChartUpdate
	s.mu.RUnlock()

	for _, handler := range handlers {
		handler(*chartUpdate)
	}
}

// SubscribeQuote subscribes to real-time quote data for a symbol
func (s *DataSubscriber) SubscribeQuote(symbol interface{}) error {
	endpoint := "md/subscribequote"
	params := map[string]interface{}{
		"symbol": symbol,
	}

	// Check if already subscribed
	_, exists := s.isSubscribed(endpoint, params)
	if exists {
		return nil
	}

	// Send subscription request
	if err := s.client.Send(endpoint, params); err != nil {
		return err
	}

	// Add to subscriptions
	s.addSubscription(endpoint, params, 0)

	if s.log != nil {
		s.log.Infof("Subscribed to %s for %v", endpoint, symbol)
	}
	return nil
}

// UnsubscribeQuote unsubscribes from quote data
func (s *DataSubscriber) UnsubscribeQuote(symbol interface{}) error {
	subscribeEndpoint := "md/subscribequote"
	unsubscribeEndpoint := "md/unsubscribequote"
	params := map[string]interface{}{
		"symbol": symbol,
	}

	// Get the key using subscribe endpoint (since that's what we stored it as)
	key, exists := s.isSubscribed(subscribeEndpoint, params)
	if !exists {
		if s.log != nil {
			s.log.Infof("Not subscribed to quotes for %v", symbol)
		}
		return nil
	}

	// Check if we should actually unsubscribe
	shouldUnsubscribe, _ := s.removeSubscription(key)
	if !shouldUnsubscribe {
		if s.log != nil {
			s.log.Warnf("Still have active references to quotes for %v", symbol)
		}
		return nil
	}

	// Send unsubscribe request
	if err := s.client.Send(unsubscribeEndpoint, params); err != nil {
		return err
	}

	if s.log != nil {
		s.log.Infof("Unsubscribed from quotes for %v", symbol)
	}
	return nil
}

// GetChart requests chart data (historical and/or live)
// Note: This is NOT a subscription, it's a one-time data request
func (s *DataSubscriber) GetChart(params marketdata.HistoricalDataParams) error {

	if err := s.client.Send("md/getchart", params); err != nil {
		return err
	}

	if s.log != nil {
		s.log.Infof("Requested chart data for %v", params.Symbol)
	}
	return nil
}

// SubscribeUserSyncRequests subscribes to user sync updates
func (s *DataSubscriber) SubscribeUserSyncRequests(users []int) error {
	endpoint := "user/syncrequest"
	params := map[string]interface{}{
		"users": users,
	}

	// Check if already subscribed
	_, exists := s.isSubscribed(endpoint, params)
	if exists {
		return nil
	}

	if err := s.client.Send(endpoint, params); err != nil {
		return err
	}

	s.addSubscription(endpoint, params, 0)

	if s.log != nil {
		s.log.Println("Subscribed to user sync requests")
	}
	return nil
}

// UnsubscribeAll unsubscribes from all active subscriptions
func (s *DataSubscriber) UnsubscribeAll() error {
	s.mu.Lock()
	subscriptions := make(map[string]*SubscriptionInfo)
	for k, v := range s.subscriptions {
		subscriptions[k] = v
	}
	s.mu.Unlock()

	s.log.Println("Active Subscription to Disconnect from: ", subscriptions)

	for key, info := range subscriptions {
		// Force unsubscribe by setting ref count to 1 then removing
		s.mu.Lock()
		info.RefCount = 1
		s.mu.Unlock()

		// Determine unsubscribe endpoint
		var unsubEndpoint string
		var unsubParams map[string]interface{}

		switch info.Endpoint {
		case "md/subscribequote":
			unsubEndpoint = "md/unsubscribequote"
			unsubParams = map[string]interface{}{
				"symbol": info.Params["symbol"],
			}
		default:
			// For other subscriptions, might not have an unsubscribe
			continue
		}

		// Remove from tracking
		s.removeSubscription(key)

		// Send unsubscribe
		if err := s.client.Send(unsubEndpoint, unsubParams); err != nil {
			if s.log != nil {
				s.log.Errorf("Error unsubscribing from %s: %v", info.Endpoint, err)
			}
		}
	}

	return nil
}

// GetActiveSubscriptions returns a copy of active subscriptions
func (s *DataSubscriber) GetActiveSubscriptions() map[string]*SubscriptionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subscriptions := make(map[string]*SubscriptionInfo)
	for k, v := range s.subscriptions {
		// Create a deep copy
		paramsCopy := make(map[string]interface{})
		for pk, pv := range v.Params {
			paramsCopy[pk] = pv
		}

		subscriptions[k] = &SubscriptionInfo{
			Endpoint: v.Endpoint,
			Params:   paramsCopy,
			ChartID:  v.ChartID,
			RefCount: v.RefCount,
		}
	}
	return subscriptions
}
