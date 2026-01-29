# Tradovate Execution Engine - User Guide

**Automated Trading System for Tradovate Futures**

A terminal-based execution engine built in Go featuring real-time market data, automated strategy execution, and comprehensive risk management.

---

## Quick Start

1. **Install** - See [SETUP.md](SETUP.md) for installation instructions
2. **Configure** - Get Tradovate credentials (see below)
3. **Connect** - Press `Shift + 1` to connect
4. **Trade** - Configure and start your strategy

---

## Table of Contents

- [Features](#features)
- [Getting Credentials](#getting-credentials)
- [First Time Setup](#first-time-setup)
- [Using the Interface](#using-the-interface)
- [Strategy Configuration](#strategy-configuration)
- [Commands Reference](#commands-reference)
- [Risk Management](#risk-management)
- [Logging Configuration](#logging-configuration)
- [Troubleshooting](#troubleshooting)

---

## Features

### Core Functionality
- ✅ Real-time WebSocket connection to Tradovate API
- ✅ Automated MA Crossover strategy execution
- ✅ Market order submission and tracking
- ✅ Live position and P&L monitoring
- ✅ Two-layer risk management system
- ✅ Terminal UI with 5 tabs
- ✅ Comprehensive logging and export

### UI Features
- **Main Tab**: System status, connection info
- **Strategy Tab**: Strategy selection, configuration, metrics, logs
- **Order Management Tab**: Complete order history and status
- **Positions Tab**: Open positions with live P&L, session P&L
- **Commands Tab**: Complete command reference

---

## Getting Credentials

### Step 1: Create MyFundedFutures Account

Create an account at [MyFundedFutures](https://myfundedfutures.com) to obtain your Tradovate username and password.

### Step 2: Extract Tradovate API Credentials

**For detailed step-by-step instructions on extracting Tradovate API credentials, see:**

📁 **[Tradovate Credential Extraction Guide](https://docs.google.com/document/d/16pkRcAYS2NYf8K-AFAIOAEIwCmt5l2MzJlOkV66RcIs/edit?usp=sharing)**  

This guide provides instructions for obtaining:
- `appId`
- `appVersion`
- `chl`
- `cid`
- `deviceId`
- `sec` (security token)

⚠️ **Note:** Demo credentials only work with `"environment": "demo"`. For live trading, use your own credentials.

---

## First Time Setup

### 1. Run the Application

```bash
Windows
trading-engine.exe
Linux
./trading-engine.exe
```

On first run, the application will:
1. Run automated tests
2. Create default `config/config.json`
3. Launch the TUI interface

### 2. Configure Credentials

**Method A: Built-in Editor**
1. Press `Shift + 4` (or type `:config`)
2. Edit the configuration
3. Press `Ctrl + S` to save
4. Press `Esc` to exit
5. Look for "Config saved" in System Log

**Method B: External Editor**
1. Close the application
2. Edit `config/config.json` manually
3. Restart the application

### 3. Connect to Tradovate

1. Press `Shift + 1` (the `!` key)
2. Wait for connection confirmation
3. Check System Log for:
   - "WebSocket connected"
   - "WebSocket authorized"
   - "Account ID: XXXXX retrieved"

---

## Using the Interface

### Tab Navigation

Switch between tabs:
- **Number Keys**: Press `1`, `2`, `3`, `4`, or `5`
- **A/D Keys**: Press `A` (previous tab) or `D` (next tab)

### Command Input

Press `:` (colon) to enter command mode, then type your command:

```
:command [arguments]
```

Example:
```
:strategy ma_crossover
:set symbol MESH6
:start
```

Press `Enter` to execute. Press `Esc` to cancel.

### Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Shift + 1` (`!`) | Connect to Tradovate API |
| `Shift + 2` (`@`) | Open strategy selection (in Strategy tab) |
| `Shift + 4` (`$`) | Open config editor |
| `1` - `5` | Switch to tab 1-5 |
| `A` | Previous tab |
| `D` | Next tab |
| `:` | Enter command mode |
| `Esc` | Exit editor/command mode |
| `Ctrl + S` | Save in editor |
| `Ctrl + C` | Quit application |
| `w` | Scroll up log |
| `s` | Scroll down log |
| `Shift + w` (`W`) | Go to top of log |
| `Shift + s` (`S`) | Go to bottom of log |

---

## Strategy Configuration

### Available Strategies

Currently implemented:
- **ma_crossover** - Moving Average Crossover Strategy

### Selecting a Strategy

**Method 1: Command**
```
:strategy ma_crossover
```

**Method 2: Strategy Selection**
1. Navigate to Strategy tab (press `2`)
2. Press `Shift + 2`
3. Type `ma_crossover`

### Configuring Parameters

After selecting a strategy:

```
:set <parameter> <value>
```

**MA Crossover Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| symbol | string | MESH6 | Trading symbol |
| fast_length | int | 5 | Fast SMA period |
| slow_length | int | 15 | Slow SMA period |

**Example:**
```
:set symbol MESH6
:set fast_length 5
:set slow_length 15
```

### ⚠️ Critical: Symbol Selection

You MUST use the current front-month contract symbol:

**Incorrect:** `ES`, `NQ`, `MES` (generic symbols)  
**Correct:** `MESH6`, `NQH6`, `ESH6` (specific contract month/year)

To find valid symbols:
1. Log into Tradovate Trader
2. Search for your product (e.g., Micro E-mini S&P 500)
3. Note the full contract symbol
4. Use the front month (current or next expiring contract)

### Starting and Stopping

**Start Strategy:**
```
:start
```

**Stop Strategy:**
```
:stop
```

Status shown in Strategy tab:
- Stopped
- Starting
- Running
- Stopping
- Error

### MA Crossover Logic

**Entry Signals:**
- **Long**: Fast SMA crosses above Slow SMA
- **Short**: Fast SMA crosses below Slow SMA

**Position Sizing:**
- Fixed at 1 contract per trade

**Update Frequency:**
- 1-minute bars only (OnBarClose mode)
- Signals generated at bar close

---

## Commands Reference

### Trading Commands

| Command | Usage | Mode | Description |
|---------|-------|------|-------------|
| buy | `:buy <symbol> <qty>` | Live | Submit market buy order |
| sell | `:sell <symbol> <qty>` | Live | Submit market sell order |
| flatten | `:flatten` | Live | Close all positions |

**Visual Mode:** Manual trading commands are disabled  
**Live Mode:** All trading functionality enabled

Switch modes:
```
:mode live
:mode visual
```

### Strategy Commands

| Command | Usage | Description |
|---------|-------|-------------|
| strategy | `:strategy <name>` | Select strategy |
| set | `:set <param> <value>` | Configure parameter |
| start | `:start` | Start strategy |
| stop | `:stop` | Stop strategy |

### System Commands

| Command | Usage | Description |
|---------|-------|-------------|
| config | `:config` | Open config editor |
| mode | `:mode <live\|visual>` | Switch trading mode |
| export | `:export <log\|orders\|strat>` | Export logs |
| help | `:help` | Navigate to commands tab |
| quit | `:quit` or `:q` | Exit application |

**Export Examples:**
```
:export log        # Export main system log
:export orders     # Export order log
:export strat      # Export strategy log
```

Logs exported to: `external/logs/`

---

## Risk Management

### Risk Controls

From `config.json`:

```json
"risk": {
  "maxContracts": 1,
  "dailyLossLimit": 500,
  "enableRiskChecks": true
}
```

**maxContracts:**
- Maximum position size per symbol
- User adjustable
- Note: MA Crossover trades 1 contract regardless of this setting

**dailyLossLimit:**
- Maximum loss per day (dollars)
- User adjustable
- Triggers automatic actions when breached

**enableRiskChecks:**
- `true`: Enable all risk checks (recommended)
- `false`: Disable (⚠️ NOT RECOMMENDED)

### Automatic Risk Actions

When daily loss limit is breached:

1. **All positions flattened** immediately
2. **Strategy execution stops** automatically
3. **Error message** displayed in status bar
4. **Log entry** created in System Log

### Manual Position Exit

Emergency flatten:
```
:flatten
```

- Closes all open positions
- Uses market orders
- Works in Live mode only
- Bypasses daily loss limit check

---

## Logging Configuration

The application uses a custom in-memory logger with support for log levels.  
Log filtering is enforced **inside the logger**, not at call sites.


### Logger Initialization

Log level is defined once at startup and passed into all logger instances:
```go
logLevel := logger.LevelInfo

mainLog     := logger.NewLogger(500, logLevel)
orderLog    := logger.NewLogger(500, logLevel)
strategyLog := logger.NewLogger(500, logLevel)
```

**Note:** This initialization currently occurs in `UIPanel.go` (around line 107).

### Supported Log Levels

| Level | Priority | Description |
|-------|----------|-------------|
| `DEBUG` | 0 | Diagnostic and verbose output |
| `INFO` | 1 | Normal operational events |
| `WARN` | 2 | Unexpected but recoverable conditions |
| `ERROR` | 3 | Failures requiring attention |


### Log Level Behavior

Each logger enforces a minimum log level. Messages below the configured level are silently dropped.

**Examples:**
- `minLevel = INFO` → `DEBUG` logs are ignored
- `minLevel = DEBUG` → All logs are recorded

This behavior is implemented inside the logger's internal `log()` method, ensuring:
- ✅ No conditional logic at call sites
- ✅ No commented-out logs
- ✅ Clear semantic intent between `INFO` and `DEBUG`


### Changing Log Levels

To enable debug logging globally, change the startup value:
```go
logLevel := logger.LevelDebug
```

All logger instances will immediately begin recording debug-level messages.

---

## Troubleshooting

### Connection Issues

**"Failed to connect to WebSocket"**
- Check internet connection
- Verify environment setting matches account type
- Check firewall/antivirus settings
- Verify Tradovate API is online

**"Authorization failed"**
- Verify all credentials are correct
- Ensure credentials haven't expired
- Try demo credentials first
- Check environment matches account type

**"Not authenticated" when placing orders**
- Reconnect using `Shift + 1`
- Check System Log for errors
- Restart application if persistent

### Strategy Issues

**Strategy not generating signals**
- Verify symbol is correct and market is open
- Check `fast_length < slow_length`
- Ensure strategy is started (`:start`)
- Wait for sufficient bars (need at least `slow_length` bars)
- Check Strategy Log for errors

**Orders rejected**
- Check if daily loss limit exceeded
- Verify max contracts not exceeded
- Ensure sufficient account balance
- Review Order Management tab for reason

**Strategy shows "Disabled"**
- Strategy needs to be started with `:start`
- Check for error messages
- Verify connection is active

### Data Issues

**No market data updating**
- Verify market is open (futures trade nearly 24/5)
- Check WebSocket connection status
- Ensure symbol is correct
- Try reconnecting

**P&L not updating**
- Verify positions exist
- Check market data is flowing
- Look for errors in System Log

**Orders not appearing**
- Wait a few seconds for confirmation
- Check System Log for submission errors
- Verify connection to trading WebSocket

---

## Important Notes

### ⚠️ Critical Warnings

1. **DO NOT trade manually on Tradovate platform while engine is running**
   - Engine maintains internal state
   - External trades will cause discrepancies
   - Use engine's flatten command if needed

2. **Session Realized P&L does not include fees/commissions**
   - Displays gross P&L only
   - Refer to Tradovate account statement for net P&L

3. **Export logs before closing application**
   - Logs stored in memory only (500 entry limit)
   - No automatic persistence

### Environment Settings

**Demo Environment:**
```json
"environment": "demo"
```
- Simulation/paper trading
- No real money at risk
- Safe for testing

**Live Environment:**
```json
"environment": "live"
```
- Real money trading
- All trades are binding
- ⚠️ Use with caution

### Trading Hours

Futures markets trade nearly 24/5:
- Sunday evening through Friday afternoon
- Brief maintenance windows
- Check Tradovate for specific hours

---

## Known Limitations

**By Design:**
- Single strategy only (MA Crossover)
- 1-minute bars only
- Fixed 1 contract per trade
- No partial fill handling
- Manual reconnection required
- Market orders only
- PnL works with multiple symbols works but can break strategy

**For full technical details, see [ARCHITECTURE.md](ARCHITECTURE.md)**

---

## Support

**For setup issues:** See [SETUP.md](SETUP.md)  
**For architecture details:** See [ARCHITECTURE.md](ARCHITECTURE.md)  
**For bugs/features:** GitHub Issues

---

## Project Info

**Repository:** https://github.com/10Hr/Tradovate-Execution-Engine  
**Author:** Tyler (10Hr)  
**Language:** Go 1.25.5  
**License:** See repository

---

**Built with Go | Tradovate API | Bubbletea TUI Framework**
