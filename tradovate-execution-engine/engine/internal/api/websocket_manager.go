package api

import (
	"encoding/json"
	"fmt"
	sync "sync"
	time "time"

	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"

	"github.com/gorilla/websocket"
)

// MessageHandler is a callback for processing incoming WebSocket messages
type MessageHandler func(eventType string, data json.RawMessage)

// TradovateWebSocketClient manages WebSocket connection lifecycle
type TradovateWebSocketClient struct {
	accessToken  string
	wsURL        string
	conn         *websocket.Conn
	isAuthorized bool
	mu           sync.RWMutex
	log          *logger.Logger

	// Message handler for routing events
	messageHandler MessageHandler
}

// WSResponse represents a WebSocket response from Tradovate
type WSResponse struct {
	Status     int             `json:"s,omitempty"`
	Event      string          `json:"e,omitempty"`
	Data       json.RawMessage `json:"d,omitempty"`
	StatusText string          `json:"statusText,omitempty"`
}

// NewTradovateWebSocketClient creates a new WebSocket client
func NewTradovateWebSocketClient(accessToken, environment string) *TradovateWebSocketClient {
	// Market data uses separate endpoints: md-demo and md-live
	wsURL := config.GetWSBaseURL(environment)

	return &TradovateWebSocketClient{
		accessToken: accessToken,
		wsURL:       wsURL,
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
		c.log.Infof("Connecting to WebSocket: %s", c.wsURL)
	}

	var err error
	c.conn, _, err = websocket.DefaultDialer.Dial(c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	if c.log != nil {
		c.log.Info("WebSocket connected")
	}

	// Start message handler
	go c.handleMessages()

	// Authorize the connection
	if err := c.authorize(); err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	return nil
}

// authorize sends authorization message with access token
func (c *TradovateWebSocketClient) authorize() error {
	// Wait a moment for the open frame to complete
	time.Sleep(100 * time.Millisecond)

	// Tradovate uses plain text format delimited by newlines: authorize\n1\n\n{token}
	authMsg := fmt.Sprintf("authorize\n1\n\n%s", c.accessToken)

	if c.log != nil {
		c.log.Info("Sending authorization...")
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
					c.log.Info("✓ WebSocket authorized")
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
	message := fmt.Sprintf("%s\n0\n\n%s", url, jsonBody)

	if c.log != nil {
		c.log.Infof("Sending message: %s", message)
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
			if c.log != nil {
				c.log.Info("WebSocket session opened")
			}

		case 'h':
			// Heartbeat frame - send response
			c.sendHeartbeat()

		case 'a':
			// Array frame - contains JSON data
			c.handleArrayFrame(payload)

		case 'c':
			// Close frame
			if c.log != nil {
				c.log.Infof("Server closing connection: %s", string(payload))
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
		c.conn.WriteMessage(websocket.TextMessage, []byte("[]"))
	}
}

// handleArrayFrame processes array frames containing JSON messages
func (c *TradovateWebSocketClient) handleArrayFrame(payload []byte) {
	var messages []json.RawMessage
	if err := json.Unmarshal(payload, &messages); err != nil {
		if c.log != nil {
			c.log.Errorf("Error unmarshaling array frame: %v, payload: %s", err, string(payload))
		}
		return
	}

	for _, msg := range messages {
		var response WSResponse
		if err := json.Unmarshal(msg, &response); err != nil {
			if c.log != nil {
				c.log.Errorf("Error unmarshaling message: %v", err)
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
				c.log.Errorf("Error response: Status %d - %s", response.Status, response.StatusText)
			}
		}

		// Handle event messages - delegate to message handler
		if response.Event != "" && c.messageHandler != nil {
			c.messageHandler(response.Event, response.Data)
			continue
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
			c.log.Info("Request successful")
		}
	} else {
		if c.log != nil {
			c.log.Errorf("Request failed: Status %d - %s", response.Status, response.StatusText)
		}
	}
}

// IsAuthorized returns whether the connection is authorized
func (c *TradovateWebSocketClient) IsAuthorized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isAuthorized
}

// Disconnect closes the WebSocket connection
func (c *TradovateWebSocketClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.isAuthorized = false
		return err
	}

	return nil
}
