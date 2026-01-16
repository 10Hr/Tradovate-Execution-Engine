package UI

import (
	"fmt"
	"strings"
	"time"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/logger"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true)

	activeTabStyle = tabStyle.Copy().
			Foreground(lipgloss.Color("36")).
			Background(lipgloss.Color("235"))

	inactiveTabStyle = tabStyle.Copy().
				Foreground(lipgloss.Color("240"))

	contentStyle = lipgloss.NewStyle().
			Padding(1, 2)

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)

	commandBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("234")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46"))

	logPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)
)

type Tab int

const (
	TabMain Tab = iota
	TabOrderManagement
	TabPositions
	TabOrders
	TabExecutions
	TabCommands
	configFile = "config.json"
)

type TradingMode int

const (
	ModeVisual TradingMode = iota
	ModeLive
)

type mode int

const (
	modeNormal mode = iota
	modeCommand
)

type Position struct {
	Symbol   string
	Quantity int
	AvgPrice float64
	PnL      float64
}

type Order struct {
	ID       string
	Symbol   string
	Side     string
	Quantity int
	Price    float64
	Status   string
	Time     time.Time
}

type Execution struct {
	Time     time.Time
	Symbol   string
	Side     string
	Quantity int
	Price    float64
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

type model struct {
	activeTab            Tab
	mode                 mode
	tradingMode          TradingMode
	commandInput         string
	searchInput          string
	statusMsg            string
	errorMsg             string
	width                int
	height               int
	searchActive         bool
	scrollOffset         int
	logScrollOffset      int
	orderLogScrollOffset int

	// Logger
	mainLogger  *logger.Logger
	orderLogger *logger.Logger

	// Data
	positions  []Position
	orders     []Order
	executions []Execution
	commands   []Command
	pnlHistory []PnLDataPoint

	// Connection status
	connected bool
	totalPnL  float64

	// Config
	configPath   string
	strategyName string
}

func InitialModel(mainLog, orderLog *logger.Logger) model {
	// Use provided loggers or create defaults
	if mainLog == nil {
		mainLog = logger.NewLogger(500)
	}
	if orderLog == nil {
		orderLog = logger.NewLogger(500)
	}

	// Initial log messages
	mainLog.Println("System initialized")
	mainLog.Printf("Starting trading engine v1.0.0")

	// Configure singleton TokenManager with logger
	auth.GetTokenManager().SetLogger(mainLog)

	return model{
		activeTab:    TabMain,
		mode:         modeNormal,
		tradingMode:  ModeVisual,
		connected:    false,
		totalPnL:     1234.56,
		mainLogger:   mainLog,
		orderLogger:  orderLog,
		configPath:   "/config/trading.yml",
		strategyName: "No strategy selected",

		// Sample data
		positions: []Position{
			{Symbol: "ESH5", Quantity: 2, AvgPrice: 5000.00, PnL: 250.00},
			{Symbol: "NQH5", Quantity: -1, AvgPrice: 17500.00, PnL: -125.50},
		},
		orders: []Order{
			{ID: "ORD001", Symbol: "ESH5", Side: "BUY", Quantity: 1, Price: 4995.00, Status: "WORKING", Time: time.Now().Add(-5 * time.Minute)},
			{ID: "ORD002", Symbol: "NQH5", Side: "SELL", Quantity: 2, Price: 17550.00, Status: "WORKING", Time: time.Now().Add(-3 * time.Minute)},
		},
		executions: []Execution{
			{Time: time.Now().Add(-5 * time.Minute), Symbol: "ESH5", Side: "BUY", Quantity: 2, Price: 5000.00},
			{Time: time.Now().Add(-10 * time.Minute), Symbol: "NQH5", Side: "SELL", Quantity: 1, Price: 17500.00},
		},
		pnlHistory: []PnLDataPoint{
			{Time: time.Now().Add(-30 * time.Minute), PnL: 0},
			{Time: time.Now().Add(-25 * time.Minute), PnL: 150},
			{Time: time.Now().Add(-20 * time.Minute), PnL: 300},
			{Time: time.Now().Add(-15 * time.Minute), PnL: 250},
			{Time: time.Now().Add(-10 * time.Minute), PnL: 400},
			{Time: time.Now().Add(-5 * time.Minute), PnL: 1234.56},
		},
		commands: []Command{
			{Name: "buy", Description: "Place a buy order", Usage: ":buy <symbol> @<price> qty:<quantity>", Category: "Trading"},
			{Name: "sell", Description: "Place a sell order", Usage: ":sell <symbol> @<price> qty:<quantity>", Category: "Trading"},
			{Name: "cancel", Description: "Cancel an order", Usage: ":cancel <orderID>", Category: "Trading"},
			{Name: "flatten", Description: "Flatten all positions", Usage: ":flatten", Category: "Trading"},
			{Name: "mode", Description: "Switch trading mode (live/visual)", Usage: ":mode <live|visual>", Category: "System"},
			{Name: "config", Description: "Edit configuration", Usage: ":config", Category: "System"},
			{Name: "strategy", Description: "Select strategy", Usage: ":strategy <name>", Category: "System"},
			{Name: "export", Description: "Export logs", Usage: ":export <log|orders>", Category: "System"},
			{Name: "main", Description: "Switch to main hub", Usage: ":main", Category: "Navigation"},
			{Name: "om", Description: "Switch to order management", Usage: ":om", Category: "Navigation"},
			{Name: "pos", Description: "Switch to positions tab", Usage: ":pos", Category: "Navigation"},
			{Name: "orders", Description: "Switch to orders tab", Usage: ":orders", Category: "Navigation"},
			{Name: "help", Description: "Show commands page", Usage: ":help", Category: "Navigation"},
			{Name: "quit", Description: "Exit the application", Usage: ":quit or :q", Category: "System"},
		},
	}
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		// Update data periodically if needed
		return m, tickCmd()
	}

	return m, nil
}

func (m model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keybindings
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.mode {
	case modeNormal:
		return m.handleNormalMode(msg)
	case modeCommand:
		return m.handleCommandMode(msg)
	}

	return m, nil
}

func (m model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.errorMsg = ""

	// If on commands tab and search is active, handle search input
	if m.activeTab == TabCommands && m.searchActive {
		return m.handleSearchInput(msg)
	}

	switch msg.String() {
	case "q":
		if m.activeTab != TabMain {
			return m, tea.Quit
		}

	case "a":
		if m.activeTab > 0 {
			m.activeTab--
			m.scrollOffset = 0
		}

	case "d":
		if m.activeTab < TabCommands {
			m.activeTab++
			m.scrollOffset = 0
		}

	case "1":
		m.activeTab = TabMain
		m.scrollOffset = 0

	case "2":
		m.activeTab = TabOrderManagement
		m.scrollOffset = 0

	case "3":
		m.activeTab = TabPositions
		m.scrollOffset = 0

	case "4":
		m.activeTab = TabOrders
		m.scrollOffset = 0

	case "5":
		m.activeTab = TabExecutions
		m.scrollOffset = 0

	case "6":
		m.activeTab = TabCommands
		m.scrollOffset = 0

	case ":", "/":
		m.mode = modeCommand
		m.commandInput = ":"

	// Shift Commands (Main Menu Actions)
	case "!": // Shift+1
		if m.connected {
			m.mainLogger.Println(">>> DISCONNECTING FROM API... <<<")
			// TODO: Add proper disconnect logic to TokenManager
			m.connected = false
			m.mainLogger.Println(">>> DISCONNECTED <<<")
			m.logScrollOffset = 1000000
		} else {
			m.mainLogger.Println(">>> STARTING API CONNECTION <<<")
			m.logScrollOffset = 1000000

			cfg, err := config.LoadConfig(configFile)
			if err != nil {
				m.mainLogger.Printf("Error loading config: %v", err)
				m.mainLogger.Println("Creating default config file...")
				if err := config.CreateDefaultConfig(configFile); err != nil {
					m.mainLogger.Printf("Error creating default config: %v", err)
					break
				}
				m.mainLogger.Printf("Default config created at %s", configFile)
				break
			}
			m.mainLogger.Println("Config Loaded.")

			m.mainLogger.Println("Authenticating...")

			// Get the global token manager
			tm := auth.GetTokenManager()

			// Set credentials
			tm.SetCredentials(
				cfg.Tradovate.AppID,
				cfg.Tradovate.AppVersion,
				cfg.Tradovate.Chl,
				cfg.Tradovate.Cid,
				cfg.Tradovate.DeviceID,
				cfg.Tradovate.Environment,
				cfg.Tradovate.Username,
				cfg.Tradovate.Password,
				cfg.Tradovate.Sec,
				cfg.Tradovate.Enc,
			)

			// Authenticate
			if err := tm.Authenticate(); err != nil {
				m.mainLogger.Printf("Authentication error: %v", err)
				break
			}
			m.mainLogger.Println(">>> AUTHENTICATION SUCCESSFUL <<<")
			m.connected = true
			m.logScrollOffset = 1000000
		}

	case "@": // Shift+2
		m.mainLogger.Println(">>> REQUESTING MARKET DATA... <<<")
		m.logScrollOffset = 1000000
		// TODO: Trigger MD subscription

	case "#": // Shift+3
		m.mode = modeCommand
		m.commandInput = ":strategy "
		m.statusMsg = "Enter strategy name..."

	case "$": // Shift+4
		m.mainLogger.Println(">>> EXPORTING MAIN LOG... <<<")
		m.logScrollOffset = 1000000

	case "%": // Shift+5
		m.mainLogger.Printf(">>> CONFIG EDITOR OPENING: %s <<<", m.configPath)
		m.logScrollOffset = 1000000

	case "^": // Shift+6
		m.mainLogger.Clear()
		m.logScrollOffset = 0
		m.mainLogger.Println(">>> LOG CLEARED <<<")

	case "ctrl+f", "f":
		// Activate search on commands page
		if m.activeTab == TabCommands {
			m.searchActive = true
			m.searchInput = ""
		}

	case "w", "up":
		// Scroll up based on context
		if m.activeTab == TabMain {
			// Calculate max scroll for Main Log to clamp "infinity"
			availableLines := m.height - 11
			if availableLines < 1 { availableLines = 1 }
			
			entriesLen := m.mainLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 { maxScroll = 0 }
			
			// Clamp if we are past the bottom (e.g. from auto-scroll)
			if m.logScrollOffset > maxScroll {
				m.logScrollOffset = maxScroll
			}

			if m.logScrollOffset > 0 {
				m.logScrollOffset--
			}
			
		} else if m.activeTab == TabOrderManagement {
			// Calculate max scroll for Order Log
			availableLines := m.height - 11
			if availableLines < 1 { availableLines = 1 }
			
			entriesLen := m.orderLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 { maxScroll = 0 }
			
			// Clamp
			if m.orderLogScrollOffset > maxScroll {
				m.orderLogScrollOffset = maxScroll
			}

			if m.orderLogScrollOffset > 0 {
				m.orderLogScrollOffset--
			}
			
		} else {
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		}

	case "s", "down":
		// Scroll down based on context
		if m.activeTab == TabMain {
			// Calculate max scroll for Main Log
			availableLines := m.height - 11
			if availableLines < 1 { availableLines = 1 }
			
			entriesLen := m.mainLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 { maxScroll = 0 }
			
			if m.logScrollOffset < maxScroll {
				m.logScrollOffset++
			}
			
		} else if m.activeTab == TabOrderManagement {
			// Calculate max scroll for Order Log
			availableLines := m.height - 11
			if availableLines < 1 { availableLines = 1 }
			
			entriesLen := m.orderLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 { maxScroll = 0 }
			
			if m.orderLogScrollOffset < maxScroll {
				m.orderLogScrollOffset++
			}

		} else {
			contentHeight := m.height - 5
			fullContent := m.renderCommandsContent()
			totalLines := len(strings.Split(fullContent, "\n"))
			maxScroll := totalLines - contentHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
		}

	case "W":
		// Go to top (Shift+W)
		if m.activeTab == TabMain {
			m.logScrollOffset = 0
		} else if m.activeTab == TabOrderManagement {
			m.orderLogScrollOffset = 0
		} else {
			m.scrollOffset = 0
		}

	case "S":
		// Go to bottom (Shift+S)
		if m.activeTab == TabMain {
			availableLines := m.height - 11
			if availableLines < 1 { availableLines = 1 }
			
			entriesLen := m.mainLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 { maxScroll = 0 }
			
			m.logScrollOffset = maxScroll
			
		} else if m.activeTab == TabOrderManagement {
			availableLines := m.height - 11
			if availableLines < 1 { availableLines = 1 }
			
			entriesLen := m.orderLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 { maxScroll = 0 }
			
			m.orderLogScrollOffset = maxScroll
			
		} else {
			contentHeight := m.height - 5
			fullContent := m.renderCommandsContent()
			totalLines := len(strings.Split(fullContent, "\n"))
			maxScroll := totalLines - contentHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset = maxScroll
		}

	default:
		// Could handle other keys here
	}

	return m, nil
}

func (m model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchActive = false
		m.searchInput = ""

	case tea.KeyCtrlA:
		// Select all - not needed for our simple search, but we'll ignore it
		return m, nil

	case tea.KeyCtrlC:
		// Copy - in a terminal this would be handled by the terminal itself
		return m, nil

	case tea.KeyCtrlV:
		// Paste - in a terminal this would be handled by the terminal itself
		return m, nil

	case tea.KeyBackspace:
		if len(m.searchInput) > 0 {
			m.searchInput = m.searchInput[:len(m.searchInput)-1]
		}

	default:
		// Allow all printable characters including space
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.searchInput += msg.String()
		}
	}

	return m, nil
}

func (m model) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		m.commandInput = ""
		m.errorMsg = ""

	case tea.KeyEnter:
		var cmd tea.Cmd
		m, cmd = m.executeCommand()
		m.mode = modeNormal
		m.commandInput = ""
		return m, cmd

	case tea.KeyBackspace:
		if len(m.commandInput) > 1 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}

	default:
		if msg.Type == tea.KeyRunes {
			m.commandInput += msg.String()
		}
	}

	return m, nil
}

func (m model) executeCommand() (model, tea.Cmd) {
	cmd := strings.TrimPrefix(m.commandInput, ":")
	parts := strings.Fields(cmd)

	if len(parts) == 0 {
		return m, nil
	}

	switch parts[0] {
	case "q", "quit":
		return m, tea.Quit

	case "buy", "sell":
		if m.tradingMode != ModeLive {
			m.errorMsg = "Cannot place orders in Visual mode. Switch to Live mode with :mode live"
			m.mainLogger.Errorf("Order rejected: Not in Live mode")
			return m, nil
		}
		if len(parts) < 2 {
			m.errorMsg = "Usage: :buy <symbol> @<price> qty:<quantity>"
			return m, nil
		}
		symbol := parts[1]
		m.statusMsg = successStyle.Render(fmt.Sprintf("✓ %s order placed for %s", strings.ToUpper(parts[0]), symbol))
		m.mainLogger.Printf("%s order placed for %s", strings.ToUpper(parts[0]), symbol)
		m.orderLogger.Printf("%s %s - Price: Market, Qty: 1", strings.ToUpper(parts[0]), symbol)

	case "cancel":
		if m.tradingMode != ModeLive {
			m.errorMsg = "Cannot cancel orders in Visual mode"
			m.mainLogger.Errorf("Cancel rejected: Not in Live mode")
			return m, nil
		}
		if len(parts) < 2 {
			m.errorMsg = "Usage: :cancel <orderID>"
			return m, nil
		}
		orderID := parts[1]
		m.statusMsg = successStyle.Render(fmt.Sprintf("✓ Order %s cancelled", orderID))
		m.mainLogger.Printf("Order %s cancelled", orderID)
		m.orderLogger.Printf("CANCEL %s", orderID)

	case "flatten":
		if m.tradingMode != ModeLive {
			m.errorMsg = "Cannot flatten in Visual mode"
			m.mainLogger.Errorf("Flatten rejected: Not in Live mode")
			return m, nil
		}
		m.statusMsg = successStyle.Render("✓ All positions flattened")
		m.mainLogger.Println("All positions flattened")
		m.orderLogger.Println("FLATTEN - All positions closed")

	case "mode":
		if len(parts) < 2 {
			m.errorMsg = "Usage: :mode <live|visual>"
			return m, nil
		}
		switch strings.ToLower(parts[1]) {
		case "live":
			m.tradingMode = ModeLive
			m.statusMsg = successStyle.Render("✓ Switched to LIVE mode")
			m.mainLogger.Println("Switched to LIVE trading mode")
		case "visual":
			m.tradingMode = ModeVisual
			m.statusMsg = "Switched to VISUAL mode"
			m.mainLogger.Println("Switched to VISUAL mode")
		default:
			m.errorMsg = "Invalid mode. Use 'live' or 'visual'"
		}

	case "config":
		m.statusMsg = "Opening config editor..."
		m.mainLogger.Printf("Config file: %s", m.configPath)

	case "strategy":
		if len(parts) < 2 {
			m.errorMsg = "Usage: :strategy <name>"
			return m, nil
		}
		m.strategyName = parts[1]
		m.statusMsg = successStyle.Render(fmt.Sprintf("✓ Strategy set to: %s", m.strategyName))
		m.mainLogger.Printf("Strategy selected: %s", m.strategyName)

	case "export":
		if len(parts) < 2 {
			m.errorMsg = "Usage: :export <log|orders>"
			return m, nil
		}
		switch parts[1] {
		case "log":
			m.statusMsg = successStyle.Render("✓ Main log exported to trading_log.txt")
			m.mainLogger.Println("Main log exported")
		case "orders":
			m.statusMsg = successStyle.Render("✓ Order log exported to orders_log.txt")
			m.mainLogger.Println("Order log exported")
		default:
			m.errorMsg = "Invalid export target. Use 'log' or 'orders'"
		}

	case "main":
		m.activeTab = TabMain
		m.statusMsg = "Switched to Main Hub"

	case "om":
		m.activeTab = TabOrderManagement
		m.statusMsg = "Switched to Order Management"

	case "pos":
		m.activeTab = TabPositions
		m.statusMsg = "Switched to Positions"

	case "orders":
		m.activeTab = TabOrders
		m.statusMsg = "Switched to Orders"

	case "help":
		m.activeTab = TabCommands
		m.statusMsg = "Switched to Commands"

	default:
		m.errorMsg = fmt.Sprintf("Unknown command: %s", parts[0])
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Tabs
	tabs := m.renderTabs()

	// Content area
	content := m.renderContent()

	// Status bar
	statusBar := m.renderStatusBar()

	// Command bar
	commandBar := m.renderCommandBar()

	// Combine everything
	return lipgloss.JoinVertical(
		lipgloss.Left,
		tabs,
		content,
		statusBar,
		commandBar,
	)
}

func (m model) renderTabs() string {
	tabs := []string{}

	tabNames := []string{"Main", "Order Mgmt", "Positions", "Orders", "Executions", "Commands"}
	for i, name := range tabNames {
		style := inactiveTabStyle
		if Tab(i) == m.activeTab {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(fmt.Sprintf("%d:%s", i+1, name)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m model) renderContent() string {
	contentHeight := m.height - 5

	var content string
	switch m.activeTab {
	case TabMain:
		content = m.renderMainHub(contentHeight)
		return content
	case TabOrderManagement:
		content = m.renderOrderManagement(contentHeight)
		return content
	case TabPositions:
		content = m.renderPositions()
	case TabOrders:
		content = m.renderOrders()
	case TabExecutions:
		content = m.renderExecutions()
	case TabCommands:
		content = m.renderCommandsScrollable(contentHeight)
		return contentStyle.Width(m.width - 4).Render(content)
	}

	return contentStyle.
		Width(m.width - 4).
		Height(contentHeight).
		Render(content)
}

func (m model) renderMainHub(contentHeight int) string {
	// Total available width
	availableWidth := m.width

	// Split 40/60 roughly, but ensure we fit
	leftWidth := (availableWidth * 4) / 10
	rightWidth := availableWidth - leftWidth

	// Adjust for borders/padding
	// contentStyle adds 2 horizontal padding, so let's respect that "safe area"
	// effectively we want the main hub to span the full width minus a small margin
	leftWidth -= 2
	rightWidth -= 2

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	// Left panel - Menu
	var leftPanel strings.Builder

	// Header Section (Connection & Mode)
	connColor := "46" // Green
	connText := "CONNECTED"
	if !m.connected {
		connColor = "196" // Red
		connText = "DISCONNECTED"
	}

	modeText := "VISUAL"
	modeColor := "39" // Blue
	if m.tradingMode == ModeLive {
		modeText = "LIVE TRADING"
		modeColor = "196" // Red
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Foreground(lipgloss.Color(connColor)).Bold(true).Render("● "+connText),
		"  |  ",
		lipgloss.NewStyle().Foreground(lipgloss.Color(modeColor)).Bold(true).Render(modeText),
	)

	leftPanel.WriteString(header + "\n\n")
	leftPanel.WriteString(fmt.Sprintf("Strategy: %s\n", m.strategyName))
	leftPanel.WriteString(strings.Repeat("─", leftWidth-4) + "\n")

	// Menu Items (Compact)
	connectDesc := "Connect to API"
	if m.connected {
		connectDesc = "Disconnect from API"
	}

	menuItems := []struct {
		key  string
		desc string
	}{
		{"!", connectDesc},
		{"@", "Receive Market Data"},
		{"#", "Select Strategy"},
		{"$", "Export Main Log"},
		{"%", "Edit Config"},
		{"^", "Clear Log"},
		{"", ""},
		{"M", "Main"},
		{"O", "Order Mgmt"},
		{"P", "Positions"},
		{"T", "Trades"},
	}

	for _, item := range menuItems {
		if item.key == "" {
			leftPanel.WriteString("\n")
			continue
		}
		leftPanel.WriteString(fmt.Sprintf("%s %s\n",
			menuItemStyle.Width(3).Render("["+item.key+"]"),
			item.desc,
		))
	}

	leftContent := lipgloss.NewStyle().
		Width(leftWidth).
		Height(contentHeight).
		Padding(1).
		Render(leftPanel.String())

	// Right panel - Logger with scroll
	// Pass the exact calculation for width to avoid wrapping issues
	rightContent := m.renderLogPanel(rightWidth, contentHeight, "System Log", m.mainLogger, &m.logScrollOffset)

	// Combine panels
	return lipgloss.JoinHorizontal(lipgloss.Top, leftContent, rightContent)
}

func (m model) renderLogPanel(width, height int, title string, log *logger.Logger, scrollOffset *int) string {
	var logContent strings.Builder

	entries := log.GetEntries()

	// Layout Math:
	// Total Height = height
	// Border = 2 lines
	// Inner Height = height - 2
	// Title = 1 line
	// Separator = 1 line
	// Footer (Scroll status) = 1 line (effectively 2 with the newline separation)

	innerHeight := height - 2
	if innerHeight < 4 {
		innerHeight = 4
	} // Minimum safe height

	headerHeight := 2
	footerHeight := 2 // \n + text

	availableLines := innerHeight - headerHeight - footerHeight
	if availableLines < 1 {
		availableLines = 1
	}

	// Calculate max scroll
	maxScroll := len(entries) - availableLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if *scrollOffset > maxScroll {
		*scrollOffset = maxScroll
	}
	if *scrollOffset < 0 {
		*scrollOffset = 0
	}

	// Get visible entries
	startIdx := *scrollOffset
	endIdx := *scrollOffset + availableLines
	if endIdx > len(entries) {
		endIdx = len(entries)
	}
	if startIdx >= len(entries) {
		startIdx = len(entries) - 1
		if startIdx < 0 {
			startIdx = 0
		}
	}

	visibleEntries := []logger.LogEntry{}
	if startIdx < len(entries) {
		visibleEntries = entries[startIdx:endIdx]
	}

	// Render log entries
	logWidth := width - 4 // Border (2) + Padding (2)
	if logWidth < 20 {
		logWidth = 20
	}

	for _, entry := range visibleEntries {
		timeStr := entry.Timestamp.Format("15:04:05")
		levelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

		switch entry.Level {
		case logger.LevelError:
			levelStyle = errorStyle
		case logger.LevelWarn:
			levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		case logger.LevelInfo:
			levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
		case logger.LevelDebug:
			levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		}

		// Truncate message if too long
		msg := entry.Message
		// Time(8) + space + Level(5) + space + msg
		prefixLen := 15
		maxMsgLen := logWidth - prefixLen
		if maxMsgLen < 10 {
			maxMsgLen = 10
		}
		if len(msg) > maxMsgLen {
			msg = msg[:maxMsgLen-3] + "..."
		}

		logContent.WriteString(fmt.Sprintf("%s %s %s\n",
			timeStr,
			levelStyle.Render(fmt.Sprintf("%-5s", entry.Level)),
			msg))
	}

	// Fill empty lines to maintain height stability if not enough entries
	linesRendered := len(visibleEntries)
	if linesRendered < availableLines {
		logContent.WriteString(strings.Repeat("\n", availableLines-linesRendered))
	}

	// Add scroll indicator
	scrollPercent := 0.0
	if maxScroll > 0 {
		scrollPercent = float64(*scrollOffset) / float64(maxScroll) * 100
	}
	indicator := fmt.Sprintf("[%d/%d %.0f%%]", *scrollOffset+1, len(entries), scrollPercent)
	if len(entries) <= availableLines {
		indicator = "[All]"
	}

	logContent.WriteString("\n" + lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		Align(lipgloss.Right).
		Width(logWidth).
		Render(indicator))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Width(width - 4). // Inner width
		Align(lipgloss.Center)

	panel := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render(fmt.Sprintf("%s (%d)", title, len(entries))),
		strings.Repeat("─", width-4),
		logContent.String(),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(width-2).      // width - border
		Height(innerHeight). // height - border
		Padding(0, 1).       // Padding for readability
		Render(panel)
}

func (m model) renderOrderManagement(contentHeight int) string {
	// Total available width
	availableWidth := m.width

	// Split 40/60
	leftWidth := (availableWidth * 4) / 10
	rightWidth := availableWidth - leftWidth

	// Adjust for borders/padding
	leftWidth -= 2
	rightWidth -= 2

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	// Left panel - Metrics and Chart
	var leftPanel strings.Builder

	// Mode indicator
	modeText := "VISUAL MODE - Read Only"
	modeColor := "39"
	if m.tradingMode == ModeLive {
		modeText = "LIVE MODE - Trading Active"
		modeColor = "196"
	}
	leftPanel.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(modeColor)).
		Bold(true).
		Render(modeText))
	leftPanel.WriteString("\n\n")

	// Key metrics
	leftPanel.WriteString("═══ KEY METRICS ═══\n\n")

	pnlStyle := successStyle
	if m.totalPnL < 0 {
		pnlStyle = errorStyle
	}

	leftPanel.WriteString(fmt.Sprintf("Total P&L:      %s\n", pnlStyle.Render(fmt.Sprintf("$%.2f", m.totalPnL))))
	leftPanel.WriteString(fmt.Sprintf("Open Positions: %d\n", len(m.positions)))
	leftPanel.WriteString(fmt.Sprintf("Active Orders:  %d\n", len(m.orders)))
	leftPanel.WriteString(fmt.Sprintf("Today's Trades: %d\n\n", len(m.executions)))
	// Simple P&L chart
	leftPanel.WriteString("═══ P&L CHART ═══\n\n")

	// Use calculated width for chart
	chartWidth := leftWidth - 4
	if chartWidth < 20 {
		chartWidth = 20
	}
	leftPanel.WriteString(m.renderSimplePnLChart(chartWidth))

	if m.tradingMode == ModeLive {
		leftPanel.WriteString("\n\n═══ LIVE ACTIONS ═══\n\n")
		leftPanel.WriteString(menuItemStyle.Render("Commands:\n"))
		leftPanel.WriteString("  :buy <symbol>\n")
		leftPanel.WriteString("  :sell <symbol>\n")
		leftPanel.WriteString("  :cancel <id>\n")
		leftPanel.WriteString("  :flatten\n")
	}

	leftContent := lipgloss.NewStyle().
		Width(leftWidth).
		Height(contentHeight).
		Padding(1).
		Render(leftPanel.String())

	// Right panel - Order Logger
	rightContent := m.renderLogPanel(rightWidth, contentHeight, "Order Log", m.orderLogger, &m.orderLogScrollOffset)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftContent, rightContent)
}
func (m model) renderSimplePnLChart(width int) string {
	if len(m.pnlHistory) == 0 {
		return "No data"
	}
	var chart strings.Builder
	chartHeight := 8

	// Ensure we have reasonable width
	if width < 20 {
		width = 20
	}

	// Limit chart points to fit width
	maxPoints := width - 10
	if maxPoints < 5 {
		maxPoints = 5
	}

	points := m.pnlHistory
	if len(points) > maxPoints {
		// Sample points evenly
		step := len(points) / maxPoints
		sampledPoints := []PnLDataPoint{}
		for i := 0; i < len(points); i += step {
			sampledPoints = append(sampledPoints, points[i])
		}
		points = sampledPoints
	}

	// Find min and max
	minPnL := points[0].PnL
	maxPnL := points[0].PnL
	for _, point := range points {
		if point.PnL < minPnL {
			minPnL = point.PnL
		}
		if point.PnL > maxPnL {
			maxPnL = point.PnL
		}
	}

	// Add some padding
	pnlRange := maxPnL - minPnL
	if pnlRange == 0 {
		pnlRange = 1
	}

	// Simple ASCII chart
	for i := chartHeight; i >= 0; i-- {
		threshold := minPnL + (float64(i)/float64(chartHeight))*pnlRange

		if i == chartHeight {
			chart.WriteString(fmt.Sprintf("%7.0f │", maxPnL))
		} else if i == 0 {
			chart.WriteString(fmt.Sprintf("%7.0f │", minPnL))
		} else {
			chart.WriteString("        │")
		}

		for _, point := range points {
			if point.PnL >= threshold {
				if point.PnL >= 0 {
					chart.WriteString(successStyle.Render("█"))
				} else {
					chart.WriteString(errorStyle.Render("█"))
				}
			} else {
				chart.WriteString(" ")
			}
		}
		chart.WriteString("\n")
	}

	chart.WriteString("        └")
	chart.WriteString(strings.Repeat("─", len(points)))
	chart.WriteString("\n")

	return chart.String()
}
func (m model) renderPositions() string {
	if len(m.positions) == 0 {
		return "No positions"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-10s %8s %12s %12s\n", "Symbol", "Qty", "Avg Price", "P&L"))
	sb.WriteString(strings.Repeat("─", 50) + "\n")

	for _, pos := range m.positions {
		pnlStyle := successStyle
		if pos.PnL < 0 {
			pnlStyle = errorStyle
		}
		sb.WriteString(fmt.Sprintf("%-10s %8d %12.2f %s\n",
			pos.Symbol,
			pos.Quantity,
			pos.AvgPrice,
			pnlStyle.Render(fmt.Sprintf("$%.2f", pos.PnL)),
		))
	}

	return sb.String()
}
func (m model) renderOrders() string {
	if len(m.orders) == 0 {
		return "No active orders"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-8s %-10s %6s %8s %12s %10s %s\n", "ID", "Symbol", "Side", "Qty", "Price", "Status", "Time"))
	sb.WriteString(strings.Repeat("─", 70) + "\n")

	for _, order := range m.orders {
		sb.WriteString(fmt.Sprintf("%-8s %-10s %6s %8d %12.2f %10s %s\n",
			order.ID,
			order.Symbol,
			order.Side,
			order.Quantity,
			order.Price,
			order.Status,
			order.Time.Format("15:04:05"),
		))
	}

	return sb.String()
}
func (m model) renderExecutions() string {
	if len(m.executions) == 0 {
		return "No executions"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-20s %-10s %6s %8s %12s\n", "Time", "Symbol", "Side", "Qty", "Price"))
	sb.WriteString(strings.Repeat("─", 60) + "\n")

	for _, exec := range m.executions {
		sb.WriteString(fmt.Sprintf("%-20s %-10s %6s %8d %12.2f\n",
			exec.Time.Format("15:04:05"),
			exec.Symbol,
			exec.Side,
			exec.Quantity,
			exec.Price,
		))
	}

	return sb.String()
}
func (m model) renderCommandsContent() string {
	var sb strings.Builder
	// Search bar
	searchBarStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Padding(0, 1).
		Width(m.width - 6)

	searchPrompt := "Search: "
	if m.searchActive {
		sb.WriteString(searchBarStyle.Render(searchPrompt + m.searchInput + "█"))
	} else {
		sb.WriteString(searchBarStyle.Render(searchPrompt + m.searchInput + " (Press 'f' to search)"))
	}
	sb.WriteString("\n\n")

	// Filter commands based on search
	filteredCommands := m.commands
	if m.searchInput != "" {
		filteredCommands = []Command{}
		searchLower := strings.ToLower(m.searchInput)
		for _, cmd := range m.commands {
			if strings.Contains(strings.ToLower(cmd.Name), searchLower) ||
				strings.Contains(strings.ToLower(cmd.Description), searchLower) ||
				strings.Contains(strings.ToLower(cmd.Category), searchLower) {
				filteredCommands = append(filteredCommands, cmd)
			}
		}
	}

	if len(filteredCommands) == 0 {
		sb.WriteString("No commands found matching your search.\n")
		return sb.String()
	}

	// Group commands by category
	categoryMap := make(map[string][]Command)
	for _, cmd := range filteredCommands {
		categoryMap[cmd.Category] = append(categoryMap[cmd.Category], cmd)
	}

	// Render each category
	categories := []string{"Trading", "Navigation", "System"}
	for _, category := range categories {
		cmds, exists := categoryMap[category]
		if !exists || len(cmds) == 0 {
			continue
		}

		categoryStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			MarginTop(1).
			MarginBottom(1)

		sb.WriteString(categoryStyle.Render(fmt.Sprintf("═══ %s ═══", category)))
		sb.WriteString("\n")

		for _, cmd := range cmds {
			commandStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("46")).
				Bold(true)

			usageStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))

			sb.WriteString(fmt.Sprintf("  %s\n", commandStyle.Render(cmd.Name)))
			sb.WriteString(fmt.Sprintf("    %s\n", cmd.Description))
			sb.WriteString(fmt.Sprintf("    %s\n", usageStyle.Render(cmd.Usage)))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
func (m model) renderCommandsScrollable(viewHeight int) string {
	// Render the full content
	fullContent := m.renderCommandsContent()
	// Split into lines
	lines := strings.Split(fullContent, "\n")

	// Calculate visible range
	totalLines := len(lines)
	visibleLines := viewHeight

	// Calculate max scroll - clamp immediately
	maxScroll := totalLines - visibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Clamp scroll offset before using it
	scrollOffset := m.scrollOffset
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Get visible slice
	endLine := scrollOffset + visibleLines
	if endLine > totalLines {
		endLine = totalLines
	}

	visibleContent := strings.Join(lines[scrollOffset:endLine], "\n")

	// Add scroll indicator if there's more content
	scrollIndicator := ""
	if totalLines > visibleLines {
		scrollPercentage := float64(scrollOffset) / float64(maxScroll) * 100
		if maxScroll == 0 {
			scrollPercentage = 0
		}
		scrollIndicator = fmt.Sprintf("\n\n[Scroll: %d/%d lines (%.0f%%) - Use w/s or ↑/↓ to scroll, W=top, S=bottom]",
			scrollOffset+1, totalLines, scrollPercentage)
	}

	return visibleContent + scrollIndicator
}
func (m model) renderStatusBar() string {
	connStatus := "●"
	connColor := "46" // Green
	if !m.connected {
		connColor = "196" // Red
	}
	pnlStyle := successStyle
	if m.totalPnL < 0 {
		pnlStyle = errorStyle
	}

	modeIndicator := ""
	if m.tradingMode == ModeLive {
		modeIndicator = errorStyle.Render(" [LIVE]")
	} else {
		modeIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(" [VISUAL]")
	}

	left := fmt.Sprintf("%s Connected%s", lipgloss.NewStyle().Foreground(lipgloss.Color(connColor)).Render(connStatus), modeIndicator)
	right := pnlStyle.Render(fmt.Sprintf("P&L: $%.2f", m.totalPnL))

	statusText := left + strings.Repeat(" ", m.width-len(left)-len(right)-len(modeIndicator)) + right

	if m.statusMsg != "" {
		statusText = m.statusMsg
	}

	return statusBarStyle.Width(m.width).Render(statusText)
}
func (m model) renderCommandBar() string {
	var content string
	if m.mode == modeCommand {
		content = m.commandInput
		if m.errorMsg != "" {
			content += "  " + errorStyle.Render(m.errorMsg)
		}
	} else {
		if m.activeTab == TabCommands && m.searchActive {
			content = "Searching... (Press ESC to exit search)"
		} else if m.activeTab == TabCommands {
			content = "w/s or ↑/↓ to scroll, W=top, S=bottom, f=search, :=command, q=quit"
		} else if m.activeTab == TabMain || m.activeTab == TabOrderManagement {
			content = "w/s to scroll logs, :=command, a/d or 1-6 to switch tabs, q=quit"
		} else {
			content = "Press ':' for commands, 'q' to quit, 'a/d' or '1-6' to switch tabs"
		}
	}

	return commandBarStyle.Width(m.width).Render(content)
}
