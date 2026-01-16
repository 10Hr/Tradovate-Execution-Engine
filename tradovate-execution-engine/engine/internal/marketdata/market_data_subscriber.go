package marketdata

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

// WebSocketSender is an interface for sending messages through WebSocket
type WebSocketSender interface {
	Send(url string, body interface{}) error
}

// MarketDataSubscriber manages real-time market data subscriptions
type MarketDataSubscriber struct {
	client        WebSocketSender
	subscriptions map[string]string
	mu            sync.RWMutex

	// Custom handlers
	OnQuoteUpdate func(quote Quote)
	OnChartUpdate func(chart ChartUpdate)
}

// NewMarketDataSubscriber creates a new market data subscriber
func NewMarketDataSubscriber(client WebSocketSender) *MarketDataSubscriber {
	return &MarketDataSubscriber{
		client:        client,
		subscriptions: make(map[string]string),
	}
}

// HandleEvent processes incoming market data events
func (s *MarketDataSubscriber) HandleEvent(eventType string, data json.RawMessage) {
	switch eventType {
	case EventMarketData:
		s.handleMarketData(data)
	case EventChart:
		s.handleChartData(data)
	default:
		log.Printf("Unknown event type: %s", eventType)
	}
}

// handleMarketData processes market data events (quotes)
func (s *MarketDataSubscriber) handleMarketData(data json.RawMessage) {
	quoteData, err := ParseQuoteData(data)
	if err != nil {
		log.Printf("Error unmarshaling quote data: %v", err)
		return
	}

	for _, quote := range quoteData.Quotes {
		log.Printf("Quote Update - Contract: %d, Timestamp: %s",
			quote.ContractID, quote.Timestamp)

		if bid, ok := quote.Entries["Bid"]; ok {
			log.Printf("  Bid: %.2f @ %.0f", bid.Price, bid.Size)
		}
		if offer, ok := quote.Entries["Offer"]; ok {
			log.Printf("  Offer: %.2f @ %.0f", offer.Price, offer.Size)
		}
		if trade, ok := quote.Entries["Trade"]; ok {
			log.Printf("  Last: %.2f @ %.0f", trade.Price, trade.Size)
		}

		// Call custom handler if set
		if s.OnQuoteUpdate != nil {
			s.OnQuoteUpdate(quote)
		}
	}
}

// handleChartData processes chart/tick data events
func (s *MarketDataSubscriber) handleChartData(data json.RawMessage) {
	chartUpdate, err := ParseChartData(data)
	if err != nil {
		log.Printf("Error unmarshaling chart data: %v", err)
		return
	}

	for _, chart := range chartUpdate.Charts {
		if len(chart.Ticks) > 0 {
			log.Printf("Tick data received - Chart ID: %d, Ticks: %d", chart.ID, len(chart.Ticks))
			for _, tick := range chart.Ticks {
				log.Printf("  Tick: %s - Price: %.2f, Size: %.0f",
					tick.Timestamp, tick.Price, tick.Size)
			}
		}

		if len(chart.Bars) > 0 {
			log.Printf("Bar data received - Chart ID: %d, Bars: %d", chart.ID, len(chart.Bars))
		}
	}

	// Call custom handler if set
	if s.OnChartUpdate != nil {
		s.OnChartUpdate(*chartUpdate)
	}
}

// SubscribeQuote subscribes to real-time quote data for a symbol
func (s *MarketDataSubscriber) SubscribeQuote(symbol interface{}) error {
	body := map[string]interface{}{
		"symbol": symbol,
	}

	if err := s.client.Send("md/subscribeQuote", body); err != nil {
		return err
	}

	s.mu.Lock()
	key := fmt.Sprintf("%s:%v", SubscriptionTypeQuote, symbol)
	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
	s.mu.Unlock()

	log.Printf("Subscribed to quotes for %v", symbol)
	return nil
}

// UnsubscribeQuote unsubscribes from quote data
func (s *MarketDataSubscriber) UnsubscribeQuote(symbol interface{}) error {
	body := map[string]interface{}{
		"symbol": symbol,
	}

	if err := s.client.Send("md/unsubscribeQuote", body); err != nil {
		return err
	}

	s.mu.Lock()
	key := fmt.Sprintf("%s:%v", SubscriptionTypeQuote, symbol)
	delete(s.subscriptions, key)
	s.mu.Unlock()

	log.Printf("Unsubscribed from quotes for %v", symbol)
	return nil
}

// SubscribeTickChart subscribes to real-time tick data
func (s *MarketDataSubscriber) SubscribeTickChart(symbol interface{}) error {
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
	key := fmt.Sprintf("%s:%v", SubscriptionTypeTick, symbol)
	s.subscriptions[key] = fmt.Sprintf("%v", symbol)
	s.mu.Unlock()

	log.Printf("Subscribed to tick chart for %v", symbol)
	return nil
}

// UnsubscribeChart unsubscribes from chart data
func (s *MarketDataSubscriber) UnsubscribeChart(symbol interface{}) error {
	body := map[string]interface{}{
		"symbol": symbol,
	}

	if err := s.client.Send("md/unsubscribechart", body); err != nil {
		return err
	}

	s.mu.Lock()
	key := fmt.Sprintf("%s:%v", SubscriptionTypeTick, symbol)
	delete(s.subscriptions, key)
	s.mu.Unlock()

	log.Printf("Unsubscribed from tick chart for %v", symbol)
	return nil
}

// UnsubscribeAll unsubscribes from all active subscriptions
func (s *MarketDataSubscriber) UnsubscribeAll() error {
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
			case SubscriptionTypeQuote:
				if err := s.UnsubscribeQuote(symbol); err != nil {
					log.Printf("Error unsubscribing quote for %v: %v", symbol, err)
				}
			case SubscriptionTypeTick + ":":
				if err := s.UnsubscribeChart(symbol); err != nil {
					log.Printf("Error unsubscribing chart for %v: %v", symbol, err)
				}
			}
		}
	}

	return nil
}

// GetActiveSubscriptions returns a copy of active subscriptions
func (s *MarketDataSubscriber) GetActiveSubscriptions() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subscriptions := make(map[string]string)
	for k, v := range s.subscriptions {
		subscriptions[k] = v
	}
	return subscriptions
}
