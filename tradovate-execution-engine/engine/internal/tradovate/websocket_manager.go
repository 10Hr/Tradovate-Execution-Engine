package tradovate

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"

	"github.com/gorilla/websocket"
)

// NewTradovateWebSocketClient creates a new WebSocket client
func NewTradovateWebSocketClient(accessToken, environment, wsType string) *TradovateWebSocketClient {
	// Market data uses separate endpoints: md-demo and md-live

	var wsURL string
	switch wsType {
	case "md":
		wsURL = config.GetMDWSBaseURL(environment)
	default:
		wsURL = config.GetWSBaseURL(environment)
	}

	return &TradovateWebSocketClient{
		accessToken:     accessToken,
		wsURL:           wsURL,
		openChan:        make(chan struct{}),
		pendingRequests: make(map[uint32]string),
		heartbeatStop:   make(chan struct{}),
	}
}

// SetLogger sets the logger for the WebSocket client
func (c *TradovateWebSocketClient) SetLogger(l *logger.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = l
}

// SetMessageHandler sets the callback for handling incoming messages
func (c *TradovateWebSocketClient) SetMessageHandler(handler MessageHandler) {
	c.messageHandler = handler
}

// Connect establishes WebSocket connection and authorizes
func (c *TradovateWebSocketClient) Connect() error {
	if c.log != nil {
		/*c.log.Infof*/ fmt.Printf("Connecting to WebSocket: %s", c.wsURL)
	}

	var err error
	c.conn, _, err = websocket.DefaultDialer.Dial(c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	if c.log != nil {
		/*c.log.Info*/ fmt.Println("WebSocket connected")
	}

	// Start message handler
	go c.handleMessages()

	// Start proactive heartbeat
	go c.startHeartbeat()

	// Authorize the connection
	if err := c.authorize(); err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	return nil
}

// authorize sends authorization message with access token
func (c *TradovateWebSocketClient) authorize() error {
	// Wait for the open frame to complete
	select {
	case <-c.openChan:
		// Connection is open
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for open frame")
	}

	// Tradovate uses plain text format delimited by newlines: authorize\n1\n\n{token}
	authMsg := fmt.Sprintf("authorize\n1\n\n%s", c.accessToken)

	if c.log != nil {
		/*c.log.Info*/ fmt.Println("Sending authorization...")
	}

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("websocket not connected")
	}
	err := c.conn.WriteMessage(websocket.TextMessage, []byte(authMsg))
	c.mu.Unlock()

	if err != nil {
		return err
	}

	// Wait for authorization response (with timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("authorization timeout - no response received")
		case <-ticker.C:
			c.mu.RLock()
			if c.isAuthorized {
				c.mu.RUnlock()
				if c.log != nil {
					/*c.log.Info*/ fmt.Println("✓ WebSocket authorized")
				}
				return nil
			}
			c.mu.RUnlock()
		}
	}
}

// Send sends a message through the WebSocket in Tradovate plain text format
// Format: url\nrequest_id\n\njson_body (note the double newline before body)
func (c *TradovateWebSocketClient) Send(url string, body interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	// Marshal body to JSON
	jsonBody := ""
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal body: %w", err)
		}
		jsonBody = string(jsonData)
	}

	// Format: url\nrequest_id\n\njson_body
	// Note the double \n before the body
	requestID := atomic.AddUint32(&c.nextRequestID, 1)
	c.pendingRequests[requestID] = url
	message := fmt.Sprintf("%s\n%d\n\n%s", url, requestID, jsonBody)

	if c.log != nil {
		/*c.log.Infof*/ fmt.Printf("Sending message (ID %d): %s", requestID, url)
	}

	return c.conn.WriteMessage(websocket.TextMessage, []byte(message))
}

// handleMessages processes incoming WebSocket messages
func (c *TradovateWebSocketClient) handleMessages() {
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			c.mu.RLock()
			closed := c.conn == nil
			c.mu.RUnlock()

			if closed {
				return
			}

			if c.log != nil {
				c.log.Warnf("Read error: %v", err)
			}
			return
		}

		// Parse Tradovate frame format
		if len(message) == 0 {
			continue
		}

		frameType := message[0]
		payload := message[1:]

		switch frameType {
		case 'o':
			// Open frame - connection established
			c.mu.Lock()
			select {
			case <-c.openChan:
				// Already closed
			default:
				close(c.openChan)
			}
			c.mu.Unlock()

			if c.log != nil {
				/*c.log.Info*/ fmt.Println("WebSocket session opened")
			}

		case 'h':
			// Heartbeat frame - we send our own proactive heartbeats every 2.5s
			// so we can just ignore the server's 'h' frame or log it.
			if c.log != nil {
				//c.log.Debug("Received heartbeat frame from server")
			}

		case 'a':
			// Array frame - contains JSON data
			c.handleArrayFrame(payload)

		case 'c':
			// Close frame
			if c.log != nil {
				/*c.log.Infof*/ fmt.Printf("Server closing connection: %s", string(payload))
			}
			return

		default:
			if c.log != nil {
				c.log.Warnf("Unknown frame type: %c, payload: %s", frameType, string(payload))
			}
		}
	}
}

// sendHeartbeat sends a heartbeat response to keep connection alive
func (c *TradovateWebSocketClient) sendHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if c.log != nil {
			//c.log.Debug("Sending heartbeat response []")
		}
		c.conn.WriteMessage(websocket.TextMessage, []byte("[]"))
	}
}

// handleArrayFrame processes array frames containing JSON messages
func (c *TradovateWebSocketClient) handleArrayFrame(payload []byte) {

	//fmt.Printf("\n📨 RAW RESPONSE: %s\n", string(payload))

	if c.log != nil {
		c.log.Debugf("DEBUG: Received array frame: %s", string(payload))
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(payload, &messages); err != nil {
		if c.log != nil {
			/*c.log.Errorf*/ fmt.Errorf("Error unmarshaling array frame: %v, payload: %s", err, string(payload))
		}
		return
	}

	for _, msg := range messages {

		var response WSResponse
		if err := json.Unmarshal(msg, &response); err != nil {
			if c.log != nil {
				/*c.log.Errorf*/ fmt.Errorf("Error unmarshaling message: %v", err)
			}
			continue
		}

		// Handle authorization response
		if response.Status == 200 && !c.isAuthorized {
			c.mu.Lock()
			c.isAuthorized = true
			c.mu.Unlock()
			if c.log != nil {
				c.log.Info("Authorization confirmed")
			}
			continue
		}

		// Log any errors
		if response.Status != 0 && response.Status != 200 {
			if c.log != nil {
				/*c.log.Errorf*/ fmt.Errorf("Error response: Status %d - %s", response.Status, response.StatusText)
			}
		}

		// Handle event messages - delegate to message handler
		if response.Event != "" && c.messageHandler != nil {
			c.messageHandler(response.Event, response.Data)
			continue
		}

		// If no event name but has ID, it's a response to a request
		if response.ID != 0 && c.messageHandler != nil {
			c.mu.Lock()
			url, ok := c.pendingRequests[uint32(response.ID)]
			if ok {
				delete(c.pendingRequests, uint32(response.ID))
			}
			c.mu.Unlock()

			if ok {
				c.messageHandler(url, response.Data)
				continue
			}
		}

		// Handle other responses
		if response.Status != 0 {
			c.handleResponse(response)
		}
	}
}

// handleResponse processes response messages
func (c *TradovateWebSocketClient) handleResponse(response WSResponse) {
	if response.Status == 200 {
		if c.log != nil {
			/*c.log.Infof*/ fmt.Printf("Request %d successful", response.ID)
		}
	} else {
		if c.log != nil {
			/*c.log.Errorf*/ fmt.Errorf("Request %d failed: Status %d - %s", response.ID, response.Status, response.StatusText)
		}
	}
}

// IsAuthorized returns whether the connection is authorized
func (c *TradovateWebSocketClient) IsAuthorized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isAuthorized
}

// IsConnected returns whether the WebSocket is currently connected and authorized
func (c *TradovateWebSocketClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && c.isAuthorized
}

// Disconnect closes the WebSocket connection
func (c *TradovateWebSocketClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop heartbeat
	select {
	case c.heartbeatStop <- struct{}{}:
	default:
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.isAuthorized = false
		return err
	}

	return nil
}

func (c *TradovateWebSocketClient) startHeartbeat() {
	ticker := time.NewTicker(2500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sendHeartbeat()
		case <-c.heartbeatStop:
			return
		}
	}
}
