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
			fmt.Printf("Incremented ref count for %s to %d\n", endpoint, info.RefCount)
		}
	} else {
		s.subscriptions[key] = &SubscriptionInfo{
			Endpoint: endpoint,
			Params:   params,
			ChartID:  chartID,
			RefCount: 1,
		}
		if s.log != nil {
			fmt.Printf("\nAdded new subscription: %s with params %v\n", endpoint, params)
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
			fmt.Printf("Removed subscription: %s\n", info.Endpoint)
		}
		return true, infoCopy
	}

	if s.log != nil {
		fmt.Printf("Decremented ref count for %s to %d\n", info.Endpoint, info.RefCount)
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
			fmt.Printf("\nEvent Received: %s\n", eventType)
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
		s.handleSubscriptionResponse(eventType, data)
	case "md/subscribechart", "md/unsubscribechart":
		s.handleChartSubscriptionResponse(eventType, data)
	default:
		if s.log != nil {
			fmt.Printf("Unknown event type: %s\n", eventType)
		}
	}
}

// handleSubscriptionResponse processes subscription confirmations
func (s *DataSubscriber) handleSubscriptionResponse(eventType string, data json.RawMessage) {
	if s.log != nil {
		fmt.Printf("Confirmation received for: %s\n", eventType)
	}
}

// handleChartSubscriptionResponse processes chart subscription confirmations
// and extracts the chart ID to update the subscription info
func (s *DataSubscriber) handleChartSubscriptionResponse(eventType string, data json.RawMessage) {
	if eventType == "md/subscribechart" {
		// Parse the response to get chart ID
		var response struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(data, &response); err == nil && response.ID > 0 {
			// Update any chart subscriptions with this ID
			// Note: Ideally match by symbol/params from the response
			if s.log != nil {
				fmt.Printf("Chart subscription confirmed, ID: %d\n", response.ID)
			}
		}
	}

	if s.log != nil {
		fmt.Printf("Chart confirmation received for: %s\n", eventType)
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
			fmt.Printf("Failed to parse props event: %v\n", err)
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

// handleUserEvent processes user sync events
func (s *DataSubscriber) handleUserEvent(data json.RawMessage) {
	var syncData struct {
		Orders    []json.RawMessage `json:"orders"`
		Positions []json.RawMessage `json:"positions"`
	}

	if err := json.Unmarshal(data, &syncData); err == nil {
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
			fmt.Printf("Error unmarshaling quote data: %v\n", err)
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
				fmt.Printf("Error unmarshaling chart data: %v\n", err)
			}
			return
		}
	}

	for _, chart := range chartUpdate.Charts {
		if len(chart.Ticks) > 0 && s.log != nil {
			fmt.Printf("Tick data received - Chart ID: %d, Ticks: %d\n", chart.ID, len(chart.Ticks))
		}
		if len(chart.Bars) > 0 && s.log != nil {
			fmt.Printf("Bar data received - Chart ID: %d, Bars: %d\n", chart.ID, len(chart.Bars))
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
		s.addSubscription(endpoint, params, 0)
		if s.log != nil {
			fmt.Printf("Already subscribed to %s for %v, incremented ref count\n", endpoint, symbol)
		}
		return nil
	}

	// Send subscription request
	if err := s.client.Send(endpoint, params); err != nil {
		return err
	}

	// Add to subscriptions
	s.addSubscription(endpoint, params, 0)

	if s.log != nil {
		fmt.Printf("Subscribed to %s for %v\n", endpoint, symbol)
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
			fmt.Printf("Not subscribed to quotes for %v\n", symbol)
		}
		return nil
	}

	// Check if we should actually unsubscribe
	shouldUnsubscribe, _ := s.removeSubscription(key)
	if !shouldUnsubscribe {
		if s.log != nil {
			fmt.Printf("Still have active references to quotes for %v\n", symbol)
		}
		return nil
	}

	// Send unsubscribe request
	if err := s.client.Send(unsubscribeEndpoint, params); err != nil {
		return err
	}

	if s.log != nil {
		fmt.Printf("Unsubscribed from quotes for %v\n", symbol)
	}
	return nil
}

// SubscribeChart subscribes to real-time chart data
func (s *DataSubscriber) SubscribeChart(symbol interface{}, chartDesc map[string]interface{}) error {
	endpoint := "md/subscribechart"
	params := map[string]interface{}{
		"symbol":           symbol,
		"chartDescription": chartDesc,
	}

	// Check if already subscribed
	_, exists := s.isSubscribed(endpoint, params)
	if exists {
		s.addSubscription(endpoint, params, 0)
		if s.log != nil {
			fmt.Printf("Already subscribed to chart for %v, incremented ref count\n", symbol)
		}
		return nil
	}

	// Send subscription request
	if err := s.client.Send(endpoint, params); err != nil {
		return err
	}

	// Add to subscriptions
	s.addSubscription(endpoint, params, 0)

	if s.log != nil {
		fmt.Printf("Subscribed to chart for %v with description %v\n", symbol, chartDesc)
	}
	return nil
}

// SubscribeTickChart subscribes to real-time tick data
func (s *DataSubscriber) SubscribeTickChart(symbol interface{}) error {
	chartDesc := map[string]interface{}{
		"underlyingType": "Tick",
	}
	return s.SubscribeChart(symbol, chartDesc)
}

// SubscribeMinuteChart subscribes to real-time minute bar data
func (s *DataSubscriber) SubscribeMinuteChart(symbol interface{}) error {
	chartDesc := map[string]interface{}{
		"underlyingType":  "MinuteBar",
		"elementSize":     1,
		"elementSizeUnit": "UnderlyingUnits",
	}
	return s.SubscribeChart(symbol, chartDesc)
}

// UnsubscribeChart unsubscribes from chart data
func (s *DataSubscriber) UnsubscribeChart(symbol interface{}, chartDesc map[string]interface{}) error {
	subscribeEndpoint := "md/subscribechart"
	unsubscribeEndpoint := "md/unsubscribechart"
	params := map[string]interface{}{
		"symbol":           symbol,
		"chartDescription": chartDesc,
	}

	// Get the key using subscribe endpoint
	key, exists := s.isSubscribed(subscribeEndpoint, params)
	if !exists {
		if s.log != nil {
			fmt.Printf("Not subscribed to chart for %v\n", symbol)
		}
		return nil
	}

	// Check if we should actually unsubscribe
	shouldUnsubscribe, _ := s.removeSubscription(key)
	if !shouldUnsubscribe {
		if s.log != nil {
			fmt.Printf("Still have active references to chart for %v\n", symbol)
		}
		return nil
	}

	// Send unsubscribe request
	unsubParams := map[string]interface{}{
		"symbol": symbol,
	}
	if err := s.client.Send(unsubscribeEndpoint, unsubParams); err != nil {
		return err
	}

	if s.log != nil {
		fmt.Printf("Unsubscribed from chart for %v\n", symbol)
	}
	return nil
}

// UnsubscribeTickChart unsubscribes from tick chart
func (s *DataSubscriber) UnsubscribeTickChart(symbol interface{}) error {
	chartDesc := map[string]interface{}{
		"underlyingType": "Tick",
	}
	return s.UnsubscribeChart(symbol, chartDesc)
}

// UnsubscribeMinuteChart unsubscribes from minute chart
func (s *DataSubscriber) UnsubscribeMinuteChart(symbol interface{}) error {
	chartDesc := map[string]interface{}{
		"underlyingType":  "MinuteBar",
		"elementSize":     1,
		"elementSizeUnit": "UnderlyingUnits",
	}
	return s.UnsubscribeChart(symbol, chartDesc)
}

// GetChart requests chart data (historical and/or live)
// Note: This is NOT a subscription, it's a one-time data request
func (s *DataSubscriber) GetChart(params marketdata.HistoricalDataParams) error {
	if err := s.client.Send("md/getchart", params); err != nil {
		return err
	}

	if s.log != nil {
		fmt.Printf("\nRequested chart data for %v\n", params.Symbol)
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
		s.addSubscription(endpoint, params, 0)
		if s.log != nil {
			fmt.Println("Already subscribed to user sync, incremented ref count")
		}
		return nil
	}

	if err := s.client.Send(endpoint, params); err != nil {
		return err
	}

	s.addSubscription(endpoint, params, 0)

	if s.log != nil {
		fmt.Println("Subscribed to user sync requests")
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
		case "md/subscribechart":
			unsubEndpoint = "md/unsubscribechart"
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
				fmt.Printf("Error unsubscribing from %s: %v\n", info.Endpoint, err)
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

// package tradovate

// import (
// 	"encoding/json"
// 	"fmt"
// 	"tradovate-execution-engine/engine/internal/logger"
// 	"tradovate-execution-engine/engine/internal/marketdata"
// 	//"tradovate-execution-engine/engine/internal/marketdata"
// )

// // NewDataSubscriber creates a new market data subscriber
// func NewDataSubscriber(client marketdata.WebSocketSender) *DataSubscriber {
// 	return &DataSubscriber{
// 		client:        client,
// 		subscriptions: make(map[string]string),
// 	}
// }

// // SetLogger sets the logger for the subscriber
// func (s *DataSubscriber) SetLogger(l *logger.Logger) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	s.log = l
// }

// // IsConnected returns whether the underlying client is connected
// func (s *DataSubscriber) IsConnected() bool {
// 	return s.client.IsConnected()
// }

// // Connect attempts to connect the underlying client
// func (s *DataSubscriber) Connect() error {
// 	return s.client.Connect()
// }

// // HandleEvent processes incoming market data events
// func (s *DataSubscriber) HandleEvent(eventType string, data json.RawMessage) {
// 	switch eventType {
// 	case marketdata.EventMarketData:
// 		s.handleMarketData(data)
// 	case marketdata.EventChart, "md/getchart":
// 		s.handleChartData(data)
// 	case marketdata.EventUser:
// 		if s.log != nil {
// 			/*s.log.Infof*/ fmt.Printf("Event Received: %s\n", eventType)
// 		}
// 		// User sync often contains positions and orders
// 		if s.OnUserSync != nil {
// 			s.OnUserSync(data)
// 		}
// 		s.handleUserEvent(data)
// 	case marketdata.EventOrder:
// 		if s.OnOrderUpdate != nil {
// 			s.OnOrderUpdate(data)
// 		}
// 	case marketdata.EventPosition:
// 		if s.OnPositionUpdate != nil {
// 			s.OnPositionUpdate(data)
// 		}
// 	case marketdata.EventProps:
// 		s.handlePropsEvent(data)
// 	case "md/subscribequote", "md/unsubscribequote", "md/subscribechart", "md/unsubscribechart":
// 		// These are response confirmations for requests, ignore them to avoid "Unknown event type" warnings
// 		if s.log != nil {
// 			/*s.log.Debugf*/ fmt.Printf("\nConfirmation received for: %s", eventType)
// 		}
// 	default:
// 		if s.log != nil {
// 			/*s.log.Warnf*/ fmt.Printf("Unknown event type: %s", eventType)
// 		}
// 	}
// }

// // AddQuoteHandler adds a callback for quote updates
// func (s *DataSubscriber) AddQuoteHandler(handler func(marketdata.Quote)) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	s.OnQuoteUpdate = append(s.OnQuoteUpdate, handler)
// }

// // AddChartHandler adds a callback for chart updates
// func (s *DataSubscriber) AddChartHandler(handler func(marketdata.ChartUpdate)) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	s.OnChartUpdate = append(s.OnChartUpdate, handler)
// }

// // handlePropsEvent handles incremental updates
// func (s *DataSubscriber) handlePropsEvent(data json.RawMessage) {
// 	var props struct {
// 		EntityType string          `json:"entityType"`
// 		Entity     json.RawMessage `json:"entity"`
// 	}
// 	if err := json.Unmarshal(data, &props); err != nil {
// 		if s.log != nil {
// 			/*s.log.Warnf*/ fmt.Printf("Failed to parse props event: %v", err)
// 		}
// 		return
// 	}

// 	switch props.EntityType {
// 	case "order":
// 		if s.OnOrderUpdate != nil {
// 			s.OnOrderUpdate(props.Entity)
// 		}
// 	case "position":
// 		if s.OnPositionUpdate != nil {
// 			s.OnPositionUpdate(props.Entity)
// 		}
// 	}
// }

// // handleUserEvent processes user sync events which may contain orders/positions
// func (s *DataSubscriber) handleUserEvent(data json.RawMessage) {
// 	// UserSync response structure
// 	var syncData struct {
// 		Orders    []json.RawMessage `json:"orders"`
// 		Positions []json.RawMessage `json:"positions"`
// 	}

// 	if err := json.Unmarshal(data, &syncData); err == nil {
// 		// If successfully parsed as a sync object, iterate and dispatch
// 		if s.OnOrderUpdate != nil {
// 			for _, order := range syncData.Orders {
// 				s.OnOrderUpdate(order)
// 			}
// 		}
// 		if s.OnPositionUpdate != nil {
// 			for _, pos := range syncData.Positions {
// 				s.OnPositionUpdate(pos)
// 			}
// 		}
// 		return
// 	}

// 	// Fallback: If it's not the standard sync object, pass generic data
// 	// (Though user/syncrequest usually follows that structure)
// 	if s.OnOrderUpdate != nil {
// 		s.OnOrderUpdate(data)
// 	}
// 	if s.OnPositionUpdate != nil {
// 		s.OnPositionUpdate(data)
// 	}
// }

// // handleMarketData processes market data events (quotes)
// func (s *DataSubscriber) handleMarketData(data json.RawMessage) {
// 	quoteData, err := marketdata.ParseQuoteData(data)
// 	if err != nil {
// 		if s.log != nil {
// 			/*s.log.Errorf*/ fmt.Printf("Error unmarshaling quote data: %v", err)
// 		}
// 		return
// 	}

// 	s.mu.RLock()
// 	handlers := s.OnQuoteUpdate
// 	s.mu.RUnlock()

// 	for _, quote := range quoteData.Quotes {
// 		// Call custom handlers if set
// 		for _, handler := range handlers {
// 			handler(quote)
// 		}
// 	}
// }

// // handleChartData processes chart/tick data events
// func (s *DataSubscriber) handleChartData(data json.RawMessage) {
// 	chartUpdate, err := marketdata.ParseChartData(data)
// 	if err != nil {
// 		// Try to parse as single chart (sometimes md/getchart returns this in some cases)
// 		var singleChart marketdata.Chart
// 		if err2 := json.Unmarshal(data, &singleChart); err2 == nil {
// 			chartUpdate = &marketdata.ChartUpdate{Charts: []marketdata.Chart{singleChart}}
// 		} else {
// 			if s.log != nil {
// 				/*s.log.Errorf*/ fmt.Printf("Error unmarshaling chart data: %v, payload: %s", err, string(data))
// 			}
// 			return
// 		}
// 	}

// 	for _, chart := range chartUpdate.Charts {
// 		if len(chart.Ticks) > 0 {
// 			if s.log != nil {
// 				/*s.log.Debugf*/ fmt.Printf("Tick data received - Chart ID: %d, Ticks: %d", chart.ID, len(chart.Ticks))
// 			}
// 		}

// 		if len(chart.Bars) > 0 {
// 			if s.log != nil {
// 				/*s.log.Debugf*/ fmt.Printf("Bar data received - Chart ID: %d, Bars: %d", chart.ID, len(chart.Bars))
// 			}
// 		}
// 	}

// 	s.mu.RLock()
// 	handlers := s.OnChartUpdate
// 	s.mu.RUnlock()

// 	// Call custom handlers if set
// 	for _, handler := range handlers {
// 		handler(*chartUpdate)
// 	}
// }

// // SubscribeQuote subscribes to real-time quote data for a symbol
// func (s *DataSubscriber) SubscribeQuote(symbol interface{}) error {
// 	s.mu.Lock()
// 	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeQuote, symbol)
// 	if _, exists := s.subscriptions[key]; exists {
// 		s.mu.Unlock()
// 		return nil // Already subscribed
// 	}
// 	s.mu.Unlock()

// 	body := map[string]interface{}{
// 		"symbol": symbol,
// 	}

// 	if err := s.client.Send("md/subscribequote", body); err != nil {
// 		return err
// 	}

// 	s.mu.Lock()
// 	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
// 	s.mu.Unlock()

// 	if s.log != nil {
// 		/*s.log.Infof*/ fmt.Printf("Subscribed to quotes for %v", symbol)
// 	}
// 	return nil
// }

// // UnsubscribeQuote unsubscribes from quote data
// func (s *DataSubscriber) UnsubscribeQuote(symbol interface{}) error {
// 	body := map[string]interface{}{
// 		"symbol": symbol,
// 	}

// 	if err := s.client.Send("md/unsubscribequote", body); err != nil {
// 		return err
// 	}

// 	s.mu.Lock()
// 	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeQuote, symbol)
// 	delete(s.subscriptions, key)
// 	s.mu.Unlock()

// 	if s.log != nil {
// 		/*s.log.Infof*/ fmt.Printf("Unsubscribed from quotes for %v", symbol)
// 	}
// 	return nil
// }

// // SubscribeTickChart subscribes to real-time tick data
// func (s *DataSubscriber) SubscribeTickChart(symbol interface{}) error {
// 	params := map[string]interface{}{
// 		"symbol": symbol,
// 		"chartDescription": map[string]interface{}{
// 			"underlyingType": "Tick",
// 		},
// 	}

// 	if err := s.client.Send("md/subscribechart", params); err != nil {
// 		return err
// 	}

// 	s.mu.Lock()
// 	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeTick, symbol)
// 	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
// 	s.mu.Unlock()

// 	if s.log != nil {
// 		/*s.log.Infof*/ fmt.Printf("Subscribed to tick chart for %v", symbol)
// 	}
// 	return nil
// }

// // SubscribeMinuteChart subscribes to real-time minute bar data
// func (s *DataSubscriber) SubscribeMinuteChart(symbol interface{}) error {
// 	params := map[string]interface{}{
// 		"symbol": symbol,
// 		"chartDescription": map[string]interface{}{
// 			"underlyingType":  "MinuteBar",
// 			"elementSize":     1,
// 			"elementSizeUnit": "UnderlyingUnits",
// 		},
// 	}

// 	if err := s.client.Send("md/subscribechart", params); err != nil {
// 		fmt.Print(err)
// 		return err
// 	}

// 	s.mu.Lock()
// 	key := fmt.Sprintf("%s: %v", "Minute Bars ", symbol)
// 	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
// 	s.mu.Unlock()

// 	if s.log != nil {
// 		fmt.Printf("Subscribed to minute chart for %v\n", symbol)
// 	}
// 	return nil
// }

// // UnsubscribeChart unsubscribes from chart data
// func (s *DataSubscriber) UnsubscribeChart(symbol interface{}) error {
// 	body := map[string]interface{}{
// 		"symbol": symbol,
// 	}

// 	if err := s.client.Send("md/unsubscribechart", body); err != nil {
// 		return err
// 	}

// 	s.mu.Lock()
// 	key := fmt.Sprintf("%s:%v", marketdata.SubscriptionTypeTick, symbol)
// 	delete(s.subscriptions, key)
// 	s.mu.Unlock()

// 	if s.log != nil {
// 		/*s.log.Infof*/ fmt.Printf("Unsubscribed from tick chart for %v", symbol)
// 	}
// 	return nil
// }

// // GetChart requests chart data (historical and/or live)
// // This implements the md/getchart endpoint
// func (s *DataSubscriber) GetChart(params marketdata.HistoricalDataParams) error {
// 	if err := s.client.Send("md/getchart", params); err != nil {
// 		return err
// 	}

// 	if s.log != nil {
// 		/*s.log.Infof*/ fmt.Printf("Requested chart data for %v", params.Symbol)
// 	}
// 	return nil
// }

// func (s *DataSubscriber) SubscribeUserSyncRequests(users []int) error {
// 	body := map[string]interface{}{
// 		"users": users,
// 	}

// 	if err := s.client.Send("user/syncrequest", body); err != nil {
// 		return err
// 	}

// 	if s.log != nil {
// 		/*s.log.Info*/ fmt.Println("Subscribed to user sync requests")
// 	}
// 	return nil
// }

// // UnsubscribeAll unsubscribes from all active subscriptions
// func (s *DataSubscriber) UnsubscribeAll() error {
// 	s.mu.Lock()
// 	subscriptions := make(map[string]string)
// 	for k, v := range s.subscriptions {
// 		subscriptions[k] = v
// 	}
// 	s.mu.Unlock()

// 	for key, symbol := range subscriptions {
// 		if len(key) >= 5 {
// 			subscriptionType := key[:5]
// 			switch subscriptionType {
// 			case marketdata.SubscriptionTypeQuote:
// 				if err := s.UnsubscribeQuote(symbol); err != nil {
// 					if s.log != nil {
// 						/*s.log.Errorf*/ fmt.Printf("Error unsubscribing quote for %v: %v", symbol, err)
// 					}
// 				}
// 			case marketdata.SubscriptionTypeTick + ":":
// 				if err := s.UnsubscribeChart(symbol); err != nil {
// 					if s.log != nil {
// 						/*s.log.Errorf*/ fmt.Printf("Error unsubscribing chart for %v: %v", symbol, err)
// 					}
// 				}
// 			}
// 		}
// 	}

// 	return nil
// }

// // GetActiveSubscriptions returns a copy of active subscriptions
// func (s *DataSubscriber) GetActiveSubscriptions() map[string]string {
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()

// 	subscriptions := make(map[string]string)
// 	for k, v := range s.subscriptions {
// 		subscriptions[k] = v
// 	}
// 	return subscriptions
// }
