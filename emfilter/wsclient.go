package emfilter

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RelayClient implements Model B (WS relay): it holds an outbound WebSocket
// to a disco at wss://<disco>/ws/filter, which relays queries to it and
// forwards its signed results — no inbound reachability required.
type RelayClient struct {
	Identity          *Identity
	Handler           QueryHandler
	URL               string // ws(s)://host:port/ws/filter
	ReconnectInterval time.Duration

	dialer *websocket.Dialer
	memory map[string]any

	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewRelayClient builds a client with a default 5s reconnect interval when
// reconnect <= 0.
func NewRelayClient(identity *Identity, handler QueryHandler, url string, reconnect time.Duration) *RelayClient {
	if reconnect <= 0 {
		reconnect = 5 * time.Second
	}
	return &RelayClient{
		Identity:          identity,
		Handler:           handler,
		URL:               url,
		ReconnectInterval: reconnect,
		dialer:            websocket.DefaultDialer,
		memory:            map[string]any{},
		stopCh:            make(chan struct{}),
	}
}

// RunForever connects, performs the hello handshake, and services
// query/result frames; on any error it waits ReconnectInterval and
// reconnects. It blocks until Stop is called (never returns in normal
// operation, mirroring the Erlang em_filter_server connection loop).
func (c *RelayClient) RunForever() {
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}
		if err := c.session(); err != nil {
			log.Printf("[em_filter] relay session error (%s): %v", c.URL, err)
		}
		select {
		case <-c.stopCh:
			return
		case <-time.After(c.ReconnectInterval):
		}
	}
}

// Stop ends the current session and the reconnect loop. Safe to call more
// than once.
func (c *RelayClient) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

func (c *RelayClient) session() error {
	conn, _, err := c.dialer.Dial(c.URL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(c.Identity.HelloPayload()); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	var ack map[string]any
	if err := conn.ReadJSON(&ack); err != nil {
		return fmt.Errorf("hello ack: %w", err)
	}
	if ack["action"] != "hello_ok" {
		return fmt.Errorf("hello rejected: %v", ack)
	}

	for {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			return fmt.Errorf("recv: %w", err)
		}
		if msg["action"] != "query" {
			// Anything other than a query frame (pings handled by gorilla,
			// unknown actions) is silently ignored.
			continue
		}
		qid, _ := msg["id"].(string)
		body, _ := msg["body"].(string)

		var items any
		result, newMemory, err := c.Handler(body, c.memory)
		if err != nil {
			log.Printf("[em_filter] relay handler error for query %s: %v", qid, err)
			items = []any{}
		} else {
			c.memory = newMemory
			items = result
		}

		signerID, sig := c.Identity.SignResults(normalizeResults(items))
		if err := conn.WriteJSON(map[string]any{
			"action":    "result",
			"id":        qid,
			"results":   items,
			"signer_id": signerID,
			"signature": sig,
		}); err != nil {
			return fmt.Errorf("send result: %w", err)
		}
	}
}
