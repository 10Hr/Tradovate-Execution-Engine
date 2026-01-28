package UI

import (
	"sync/atomic"
	"time"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/execution"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/tradovate"

	"github.com/charmbracelet/bubbles/textarea"
)

type Tab int
type TradingMode int
type mode int

type tickMsg time.Time

type StrategyStatus int32

type StrategyRuntime struct {
	status atomic.Int32
}

type PositionRow struct {
	Symbol   string
	Quantity int
	AvgPrice float64
	PnL      float64
}

type OrderRow struct {
	ID       string
	Symbol   string
	Side     string
	Quantity int
	Price    float64
	Status   string
	Time     time.Time
}

type Command struct {
	Name        string
	Description string
	Usage       string
	Category    string
}

type PnLDataPoint struct {
	Time time.Time
	PnL  float64
}

type StrategyState struct {
	Name        string
	Params      []execution.StrategyParam
	Instance    execution.Strategy
	Symbol      string
	Description string

	Runtime *StrategyRuntime
}

// connMsg indicates connection success/failure
type connMsg struct {
	err error
}

type connMsgSuccess struct {
	config            *config.Config
	tokenManager      *auth.TokenManager
	orderManager      *execution.OrderManager
	mdClient          *tradovate.TradovateWebSocketClient
	mdSubscriber      *tradovate.DataSubscriber
	tradingClient     *tradovate.TradovateWebSocketClient
	tradingSubscriber *tradovate.DataSubscriber
	portfolioTracker  *portfolio.PortfolioTracker
}

type editorFinishedMsg struct {
	err        error
	nextAction string // "connect" or "none"
}

// Bar aggregator - collects quotes into minute bars
type BarAggregator struct {
	currentMinute string
	open          float64
	high          float64
	low           float64
	close         float64
	firstTick     bool
}

type LastBar struct {
	Timestamp string
	Close     float64
}

type model struct {
	activeTab            Tab
	mode                 mode
	tradingMode          TradingMode
	commandInput         string
	commandHistory       []string
	historyIndex         int
	searchInput          string
	statusMsg            string
	width                int
	height               int
	searchActive         bool
	scrollOffset         int
	logScrollOffset      int
	orderLogScrollOffset int
	stratLogScrollOffset int

	// Editor
	configEditor textarea.Model
	isLogView    bool
	editorTitle  string

	// Logger
	mainLogger     *logger.Logger
	orderLogger    *logger.Logger
	strategyLogger *logger.Logger

	// Data
	positions  []PositionRow
	orders     []OrderRow
	commands   []Command
	pnlHistory []PnLDataPoint

	// Connection status
	connected        bool
	totalPnL         float64
	unrealizedPnL    float64
	realizedPnL      float64
	dailyrealizedPnL float64

	// Config
	configPath    string
	strategyName  string
	currentSymbol string

	// Strategy Management
	availableStrategies []string
	selectedStrategy    string
	currentStrategy     *StrategyState
	strategyParams      map[string]string

	// Managers
	tm *auth.TokenManager
	om *execution.OrderManager
	pt *portfolio.PortfolioTracker

	// Market Data & Auth
	marketDataClient                 *tradovate.TradovateWebSocketClient
	tradingClient                    *tradovate.TradovateWebSocketClient
	marketDataSubscriptionManager    *tradovate.DataSubscriber
	tradingClientSubscriptionManager *tradovate.DataSubscriber

	config *config.Config
}
