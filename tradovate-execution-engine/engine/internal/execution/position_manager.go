package execution

import (
	"sync"
	"time"

	"tradovate-execution-engine/engine/internal/logger"
)

// PositionTracker tracks the current position
type PositionTracker struct {
	mu       sync.RWMutex
	position *Position
	log      *logger.Logger
}

// NewPositionTracker creates a new position tracker
func NewPositionTracker(symbol string, log *logger.Logger) *PositionTracker {
	return &PositionTracker{
		position: &Position{
			Symbol:        symbol,
			Quantity:      0,
			EntryPrice:    0,
			CurrentPrice:  0,
			UnrealizedPnL: 0,
			RealizedPnL:   0,
			OpenedAt:      time.Time{},
			LastUpdated:   time.Now(),
		},
		log: log,
	}
}

// UpdateFill updates position based on a fill
func (pt *PositionTracker) UpdateFill(fill *Fill, side OrderSide) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.log.Infof("Processing fill: %s %d @ %.2f", side, fill.Quantity, fill.Price)

	fillQty := fill.Quantity
	if side == SideSell {
		fillQty = -fillQty
	}

	// Check if this is closing or reducing a position
	if pt.position.Quantity != 0 &&
		((pt.position.Quantity > 0 && fillQty < 0) ||
			(pt.position.Quantity < 0 && fillQty > 0)) {

		// Calculate realized PnL
		closingQty := min(abs(pt.position.Quantity), abs(fillQty))
		pnlPerContract := fill.Price - pt.position.EntryPrice
		if pt.position.Quantity < 0 {
			pnlPerContract = -pnlPerContract
		}
		realizedPnL := pnlPerContract * float64(closingQty)
		pt.position.RealizedPnL += realizedPnL

		pt.log.Infof("Position reduced/closed. Realized PnL: $%.2f", realizedPnL)
	}

	// Update position quantity
	oldQty := pt.position.Quantity
	pt.position.Quantity += fillQty

	// Update entry price for new or increased positions
	if (oldQty == 0 && pt.position.Quantity != 0) ||
		(oldQty > 0 && fillQty > 0) ||
		(oldQty < 0 && fillQty < 0) {

		// Calculate weighted average entry price
		if oldQty == 0 {
			pt.position.EntryPrice = fill.Price
			pt.position.OpenedAt = fill.Timestamp
		} else {
			totalCost := (pt.position.EntryPrice * float64(abs(oldQty))) +
				(fill.Price * float64(abs(fillQty)))
			pt.position.EntryPrice = totalCost / float64(abs(pt.position.Quantity))
		}
	}

	// If position is now flat, reset entry price
	if pt.position.Quantity == 0 {
		pt.position.EntryPrice = 0
		pt.position.OpenedAt = time.Time{}
		pt.log.Info("Position is now flat")
	}

	pt.position.CurrentPrice = fill.Price
	pt.position.LastUpdated = fill.Timestamp

	pt.updateUnrealizedPnL()
	pt.logPosition()
}

// UpdatePrice updates the current market price and recalculates PnL
func (pt *PositionTracker) UpdatePrice(price float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.position.CurrentPrice = price
	pt.position.LastUpdated = time.Now()
	pt.updateUnrealizedPnL()
}

// updateUnrealizedPnL calculates unrealized PnL (must be called with lock held)
func (pt *PositionTracker) updateUnrealizedPnL() {
	if pt.position.Quantity == 0 {
		pt.position.UnrealizedPnL = 0
		return
	}

	pnlPerContract := pt.position.CurrentPrice - pt.position.EntryPrice
	if pt.position.Quantity < 0 {
		pnlPerContract = -pnlPerContract
	}
	pt.position.UnrealizedPnL = pnlPerContract * float64(abs(pt.position.Quantity))
}

// GetPosition returns a copy of the current position
func (pt *PositionTracker) GetPosition() Position {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return *pt.position
}

// GetQuantity returns the current position quantity
func (pt *PositionTracker) GetQuantity() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.position.Quantity
}

// IsFlat returns true if position is flat
func (pt *PositionTracker) IsFlat() bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.position.Quantity == 0
}

// Reset resets the position to flat
func (pt *PositionTracker) Reset() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.position.Quantity = 0
	pt.position.EntryPrice = 0
	pt.position.CurrentPrice = 0
	pt.position.UnrealizedPnL = 0
	pt.position.RealizedPnL = 0
	pt.position.OpenedAt = time.Time{}
	pt.position.LastUpdated = time.Now()

	pt.log.Info("Position tracker reset")
}

// logPosition logs the current position state
func (pt *PositionTracker) logPosition() {
	if pt.position.Quantity == 0 {
		pt.log.Info("Position: FLAT")
	} else {
		direction := "LONG"
		if pt.position.Quantity < 0 {
			direction = "SHORT"
		}
		pt.log.Infof("Position: %s %d @ %.2f | Unrealized PnL: $%.2f | Realized PnL: $%.2f",
			direction,
			abs(pt.position.Quantity),
			pt.position.EntryPrice,
			pt.position.UnrealizedPnL,
			pt.position.RealizedPnL)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
