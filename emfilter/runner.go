package emfilter

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

// FilterRunner wires a Filter implementation to the mesh transport(s)
// selected by EM_FILTER_MODE, mirroring the Erlang em_filter's start_agent.
type FilterRunner struct {
	name   string
	filter Filter
	config *AgentConfig
}

// NewFilterRunner creates a new runner. config may be nil (uses all
// defaults: env vars / emergence.conf / localhost:8080).
func NewFilterRunner(name string, filter Filter, config *AgentConfig) *FilterRunner {
	if config == nil {
		config = &AgentConfig{}
	}
	return &FilterRunner{name: name, filter: filter, config: config}
}

// Mode resolves EM_FILTER_MODE: "direct", "both", or "relay" (the default —
// NAT-friendly, no inbound port required).
func Mode() string {
	switch os.Getenv("EM_FILTER_MODE") {
	case "direct":
		return "direct"
	case "both":
		return "both"
	default:
		return "relay"
	}
}

// QueryPort resolves EM_FILTER_QUERY_PORT, the Model A listen/advertise
// port (default 9600).
func QueryPort() int {
	v, err := strconv.Atoi(os.Getenv("EM_FILTER_QUERY_PORT"))
	if err != nil || v <= 0 {
		return 9600
	}
	return v
}

// AdvertiseHost resolves EM_FILTER_HOST, the host Model A advertises in its
// gossip payload (default "0.0.0.0", matching the reference SDK — set
// EM_FILTER_HOST to the agent's real reachable address for peers to dial).
func AdvertiseHost() string {
	if h := os.Getenv("EM_FILTER_HOST"); h != "" {
		return h
	}
	return "0.0.0.0"
}

// SeedOrigins converts resolved disco nodes into base origin URLs
// ("http://host:port" / "https://host:port") for GossipPusher.
func SeedOrigins(nodes []DiscoNode) []string {
	origins := make([]string, 0, len(nodes))
	for _, n := range nodes {
		scheme := "http"
		if n.TLS {
			scheme = "https"
		}
		origins = append(origins, fmt.Sprintf("%s://%s:%d", scheme, n.Host, n.Port))
	}
	return origins
}

// RelayURL builds the Model B WS relay endpoint for a disco node:
// ws(s)://host:port/ws/filter.
func RelayURL(node DiscoNode) string {
	scheme := "ws"
	if node.TLS {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s:%d/ws/filter", scheme, node.Host, node.Port)
}

// asQueryHandler adapts a Filter to a QueryHandler, shared by both
// transports.
func asQueryHandler(f Filter) QueryHandler {
	return func(body string, memory map[string]any) (any, map[string]any, error) {
		return f.Handle(body, memory)
	}
}

// Run resolves identity and the selected transport(s), then blocks forever
// (never returns in normal operation — Model B reconnects forever and
// Model A serves forever).
func (r *FilterRunner) Run() error {
	identity, err := NewIdentity(r.name, KeyDir(r.name), r.filter.Capabilities())
	if err != nil {
		return fmt.Errorf("emfilter: load identity: %w", err)
	}
	handler := asQueryHandler(r.filter)
	mode := Mode()

	log.Printf("[em_filter] starting agent %q (id=%s) in %s mode", r.name, identity.IDB64(), mode)

	switch mode {
	case "direct":
		r.runDirect(identity, handler)
	case "both":
		go r.runDirect(identity, handler)
		r.runRelay(identity, handler)
	default: // "relay"
		r.runRelay(identity, handler)
	}
	return nil
}

// runDirect starts the Model A HTTP server and gossip push loop, then
// blocks forever.
func (r *FilterRunner) runDirect(identity *Identity, handler QueryHandler) {
	port := QueryPort()
	srv, err := NewAgentServer(identity, handler, "0.0.0.0", port)
	if err != nil {
		log.Printf("[em_filter] %s: failed to start agent server: %v", r.name, err)
		return
	}
	srv.Start()
	log.Printf("[em_filter] %s: Model A listening on :%d", r.name, srv.Port())

	seeds := SeedOrigins(r.config.ResolveNodes())
	pusher := NewGossipPusher(identity, seeds, AdvertiseHost(), srv.Port(), 0)
	pusher.Start()

	select {} // Model A never stops on its own.
}

// runRelay starts one RelayClient per resolved disco node and blocks until
// they all exit (never, in normal operation).
func (r *FilterRunner) runRelay(identity *Identity, handler QueryHandler) {
	nodes := r.config.ResolveNodes()
	reconnect := time.Duration(ReconnectMs()) * time.Millisecond

	if len(nodes) == 1 {
		// Common case: run inline, no goroutine/WaitGroup overhead.
		NewRelayClient(identity, handler, RelayURL(nodes[0]), reconnect).RunForever()
		return
	}

	done := make(chan struct{})
	remaining := len(nodes)
	for _, node := range nodes {
		node := node
		go func() {
			NewRelayClient(identity, handler, RelayURL(node), reconnect).RunForever()
			done <- struct{}{}
		}()
	}
	for i := 0; i < remaining; i++ {
		<-done
	}
}
