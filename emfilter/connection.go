package emfilter

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type connection struct {
	name        string
	node        DiscoNode
	filter      Filter
	jwtToken    string
	reconnectMs int
}

func newConnection(name string, node DiscoNode, filter Filter, jwt string, reconnectMs int) *connection {
	return &connection{
		name:        name,
		node:        node,
		filter:      filter,
		jwtToken:    jwt,
		reconnectMs: reconnectMs,
	}
}

// run loops forever: connect → register → agent_hello → messages → reconnect.
// Mirrors Erlang em_filter_server: never stops, memory persists across reconnects.
func (c *connection) run() {
	delay := time.Duration(c.reconnectMs) * time.Millisecond
	// Memory persists across reconnections within this goroutine's lifetime.
	memory := map[string]any{}
	for {
		if err := c.connectOnce(memory); err != nil {
			log.Printf("[em_filter] %s connection error (%s:%d): %v",
				c.name, c.node.Host, c.node.Port, err)
		} else {
			log.Printf("[em_filter] %s disconnected from %s:%d — reconnecting",
				c.name, c.node.Host, c.node.Port)
		}
		time.Sleep(delay)
	}
}

func (c *connection) connectOnce(memory map[string]any) error {
	scheme := "ws"
	if c.node.TLS {
		scheme = "wss"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", c.node.Host, c.node.Port),
		Path:   "/ws",
	}
	if c.jwtToken != "" {
		q := u.Query()
		q.Set("token", c.jwtToken)
		u.RawQuery = q.Encode()
	}

	log.Printf("[em_filter] %s connecting to %s", c.name, u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Step 1: register
	if err := conn.WriteJSON(map[string]any{"action": "register", "name": c.name}); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	// Step 2: agent_hello (always sent, even with empty capabilities)
	if err := conn.WriteJSON(map[string]any{
		"action":       "agent_hello",
		"capabilities": c.filter.Capabilities(),
	}); err != nil {
		return fmt.Errorf("agent_hello: %w", err)
	}

	log.Printf("[em_filter] %s registered — entering message loop", c.name)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}

		var msg map[string]any
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("[em_filter] %s invalid JSON frame, skipping", c.name)
			continue
		}

		if msg["action"] != "query" {
			// registered / agent_registered acks are silently ignored.
			continue
		}

		queryID, _ := msg["id"].(string)
		if queryID == "" {
			log.Printf("[em_filter] %s query frame missing 'id', skipping", c.name)
			continue
		}
		body, _ := msg["body"].(string)
		body = strings.TrimSpace(body)

		log.Printf("[em_filter] %s query %s: %s", c.name, queryID, body)

		result, newMemory, err := c.filter.Handle(body, memory)
		if err != nil {
			log.Printf("[em_filter] %s handler error for query %s: %v", c.name, queryID, err)
			result = nil
		} else {
			memory = newMemory
		}

		if err := conn.WriteJSON(map[string]any{
			"action": "result",
			"id":     queryID,
			"data":   result,
		}); err != nil {
			return fmt.Errorf("send result: %w", err)
		}
	}
}
