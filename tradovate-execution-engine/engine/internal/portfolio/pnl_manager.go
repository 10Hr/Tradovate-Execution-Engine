package portfolio

import (
	"encoding/json"
	"fmt"
	"time"

	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
	"tradovate-execution-engine/engine/internal/tradovate"
)

// NewPLTracker creates a new P&L tracker
func NewPLTracker(log *logger.Logger) *PLTracker {
	return &PLTracker{
		entries: make(map[string]*PLEntry),
		log:     log,
	}
}

// Update updates or creates a P&L entry
func (t *PLTracker) Update(name string, pl float64, netPos int, buyPrice, lastPrice float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if entry, exists := t.entries[name]; exists {
		entry.PL = pl
		entry.NetPos = netPos
		entry.BuyPrice = buyPrice
		entry.LastPrice = lastPrice
	} else {
		t.entries[name] = &PLEntry{
			Name:      name,
			PL:        pl,
			NetPos:    netPos,
			BuyPrice:  buyPrice,
			LastPrice: lastPrice,
		}
	}
}

// GetTotal calculates total P&L across all positions
func (t *PLTracker) GetTotal() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	total := 0.0
	for _, entry := range t.entries {
		total += entry.PL
	}
	return total
}

// GetEntries returns a copy of all P&L entries
func (t *PLTracker) GetEntries() map[string]PLEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	entries := make(map[string]PLEntry)
	for k, v := range t.entries {
		entries[k] = *v
	}
	return entries
}

// PrintSummary logs the current P&L summary
func (t *PLTracker) PrintSummary() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.log != nil {
		t.log.Info("==================== P&L SUMMARY ====================")
		for name, entry := range t.entries {
			direction := "LONG"
			if entry.NetPos < 0 {
				direction = "SHORT"
			}
			t.log.Infof("%-10s | %5s %3d | Buy: $%8.2f | Last: $%8.2f | P&L: $%9.2f",
				name, direction, abs(entry.NetPos), entry.BuyPrice, entry.LastPrice, entry.PL)
		}
		t.log.Info("=====================================================")
		t.log.Infof("TOTAL P&L: $%.2f", t.GetTotal())
		t.log.Info("=====================================================")
	}
}

// SetRealizedPnL sets the realized P&L
func (t *PLTracker) SetRealizedPnL(pnl float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.realizedPnL = pnl
}

// GetRealizedPnL returns the realized P&L
func (t *PLTracker) GetRealizedPnL() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.realizedPnL
}

// NewPortfolioTracker creates a new portfolio tracker using existing clients
func NewPortfolioTracker(authClient, mdClient *tradovate.DataSubscriber, userID int, log *logger.Logger) *PortfolioTracker {
	return &PortfolioTracker{
		tradingSubsciptionManager: authClient,
		mdSubsciptionManager:      mdClient,
		plTracker:                 NewPLTracker(log),
		log:                       log,
		positions:                 make(map[int]*Position),
		contracts:                 make(map[int]string),
		products:                  make(map[string]float64),
		userID:                    userID,
	}
}

// Start initializes the portfolio tracker
func (pt *PortfolioTracker) Start(environment string) error {
	pt.mu.Lock()
	if pt.running {
		pt.mu.Unlock()
		return fmt.Errorf("Portfolio tracker already running")
	}
	pt.running = true
	pt.mu.Unlock()

	// Verify subscribers are initialized
	if pt.tradingSubsciptionManager == nil || pt.mdSubsciptionManager == nil {
		return fmt.Errorf("Subscription managers not initialized")
	}

	// Set up handlers on existing subscribers
	pt.tradingSubsciptionManager.OnUserSync = func(data json.RawMessage) {
		pt.handleUserSync(data)
	}

	pt.tradingSubsciptionManager.OnPositionUpdate = func(data json.RawMessage) {
		pt.handlePositionUpdate(data)
	}

	pt.tradingSubsciptionManager.OnCashBalanceUpdate = func(data json.RawMessage) {
		pt.handleCashBalanceUpdate(data)
	}

	pt.mdSubsciptionManager.AddQuoteHandler(pt.handleQuoteUpdate)

	// Subscribe to user sync
	if err := pt.tradingSubsciptionManager.SubscribeUserSyncRequests([]int{pt.userID}); err != nil {
		return fmt.Errorf("failed to subscribe to user sync: %w", err)
	}

	pt.log.Info("Portfolio tracker started - subscribed to user sync")
	return nil

}

// handlePositionUpdate processes real-time position updates
func (pt *PortfolioTracker) handlePositionUpdate(data json.RawMessage) {

	var pos Position
	if err := json.Unmarshal(data, &pos); err != nil {
		pt.log.Warnf("Failed to unmarshal position update: %v", err)
		return
	}

	pt.mu.Lock()
	pt.positions[pos.ContractID] = &pos
	contractName, hasContract := pt.contracts[pos.ContractID]
	pt.mu.Unlock()

	if hasContract {

		if pos.NetPos != 0 {
			pt.log.Infof("Position update for %s: NetPos=%d, Bought Price=%.2f -> Subscribing",
				contractName, pos.NetPos, pos.Bought)

			if err := pt.mdSubsciptionManager.SubscribeQuote(contractName); err != nil {
				pt.log.Warnf("Failed to subscribe to quotes for %s: %v", contractName, err)
			}

			return
		}
		// Reset P&L in tracker for this symbol
		pt.plTracker.Update(contractName, 0, 0, 0, 0)

	}
}

// handleCashBalanceUpdate processes real-time Cash Balance updates
func (pt *PortfolioTracker) handleCashBalanceUpdate(data json.RawMessage) {
	var cb CashBalance
	if err := json.Unmarshal(data, &cb); err != nil {
		pt.log.Warnf("Failed to unmarshal cash balance: %v", err)
		return
	}
	pt.plTracker.SetRealizedPnL(cb.RealizedPnL)
	pt.log.Infof("Cash Balance Update: Realized PnL = %.2f", cb.RealizedPnL)
}

// handleUserSync processes the initial user sync response
func (pt *PortfolioTracker) handleUserSync(data json.RawMessage) {

	pt.log.Info(data)
	var syncResp UserSyncData
	if err := json.Unmarshal(data, &syncResp); err != nil {
		pt.log.Errorf("Failed to unmarshal user sync: %v", err)
		return
	}

	// Check if this is the initial response with positions
	if len(syncResp.Users) == 0 {
		return
	}

	//
	pt.log.Infof("Received sync data: %d positions, %d contracts, %d products",
		len(syncResp.Positions), len(syncResp.Contracts), len(syncResp.Products))

	pt.mu.Lock()
	// Store state
	for _, contract := range syncResp.Contracts {
		pt.contracts[contract.ID] = contract.Name
	}
	for _, product := range syncResp.Products {
		pt.products[product.Name] = product.ValuePerPoint
	}

	// Set up the unified quote handler once
	pt.mu.Unlock()

	// Process each position
	for _, pos := range syncResp.Positions {
		p := pos // Local copy
		pt.mu.Lock()
		pt.positions[pos.ContractID] = &p
		pt.mu.Unlock()

		// Find contract name
		var contractName string
		for _, contract := range syncResp.Contracts {
			if contract.ID == pos.ContractID {
				contractName = contract.Name
				break
			}
		}

		if contractName != "" {
			if pos.NetPos != 0 {
				//pt.log.Infof
				pt.log.Infof("Found active position: %s (ID: %d) - NetPos: %d -> Subscribing",
					contractName, pos.ContractID, pos.NetPos)
				// Subscribe to market data
				pt.mdSubsciptionManager.SubscribeQuote(contractName)
			}
		}
	}
}

// handleQuoteUpdate processes incoming quote updates and calculates P&L
func (pt *PortfolioTracker) handleQuoteUpdate(quote marketdata.Quote) {
	pt.mu.Lock()
	pos, hasPos := pt.positions[quote.ContractID]
	contractName, hasContract := pt.contracts[quote.ContractID]
	pt.mu.Unlock()

	if !hasPos || !hasContract {
		return
	}

	// Get the trade price
	trade, ok := quote.Entries["Trade"]
	if !ok {
		return
	}

	price := trade.Price

	// Find value per point
	var vpp float64
	pt.mu.Lock()
	for pName, val := range pt.products {
		if len(pName) > 0 && len(contractName) >= len(pName) &&
			contractName[:len(pName)] == pName {
			vpp = val
			break
		}
	}
	pt.mu.Unlock()

	if vpp == 0 {
		return
	}

	// Calculate buy price
	buyPrice := pos.NetPrice
	if buyPrice == 0 {
		buyPrice = pos.PrevPrice
	}

	// Calculate P&L: (current_price - buy_price) * vpp * position_size
	pl := (price - buyPrice) * vpp * float64(pos.NetPos)

	// if pt.log != nil {
	// 	pt.log.Debugf("Quote update for %s: Price=%.2f, P&L=%.2f", contractName, price, pl)
	// }

	// Update tracker
	pt.plTracker.Update(contractName, pl, pos.NetPos, buyPrice, price)

	//pt.log.Info("Total PnL: ", pt.plTracker.GetTotal(), " PnL: ", pl, " ", price, " ", buyPrice, " ", vpp, " ", pos.NetPos)
	// Log update periodically or on significant change
	// For live UI, the plTracker.GetTotal() is what you'd watch
}

// Stop disconnects all WebSocket connections
func (pt *PortfolioTracker) Stop() error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if !pt.running {
		return nil
	}

	pt.log.Info("Stopping portfolio tracker...")

	// Only unsubscribe (main handles disconnection)
	if pt.mdSubsciptionManager != nil {
		if err := pt.mdSubsciptionManager.UnsubscribeAll(); err != nil {
			pt.log.Warnf("Error unsubscribing: %v", err)
		}
	}

	pt.running = false
	pt.log.Info("Portfolio tracker stopped")
	return nil
}

// GetPLSummary returns the current P&L summary
func (pt *PortfolioTracker) GetPLSummary() map[string]PLEntry {
	return pt.plTracker.GetEntries()
}

// GetRealizedPnL returns the realized P&L from the tracker
func (pt *PortfolioTracker) GetRealizedPnL() float64 {
	return pt.plTracker.GetRealizedPnL()
}

// GetTotalPL returns the total P&L
func (pt *PortfolioTracker) GetTotalPL() float64 {
	return pt.plTracker.GetTotal()
}

// PrintSummary prints the current P&L summary
func (pt *PortfolioTracker) PrintSummary() {
	pt.plTracker.PrintSummary()
}

// StartWithPeriodicSummary starts the tracker and prints summary at intervals
func (pt *PortfolioTracker) StartWithPeriodicSummary(environment string, interval time.Duration) error {
	if err := pt.Start(environment); err != nil {
		return err
	}

	// Start periodic summary printing
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			pt.PrintSummary()
		}
	}()
	// // Start periodic summary printing
	// go func() {
	// 	ticker := time.NewTicker(interval)
	// 	defer ticker.Stop()

	// 	// Start periodic summary printing
	// 	go func() {
	// 		ticker := time.NewTicker(interval)
	// 		defer ticker.Stop()

	// 		for range ticker.C {
	// 			pt.PrintSummary()
	// 		}
	// 	}()
	// }()

	return nil
}

// Helper function for absolute value
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
