package emfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// QueryHandler processes an incoming query. It mirrors the Filter interface:
// body is the raw query string, memory is the current memory state (empty
// map on the first call), and the return values are the result (any
// JSON-serialisable value — typically a []map[string]any of embryo items),
// the new memory state, and an optional error.
type QueryHandler func(body string, memory map[string]any) (result any, newMemory map[string]any, err error)

// AgentServer implements Model A (direct): it serves
//   - POST /agent/query — runs the handler, replies with signed results.
//   - POST /pop/gossip  — accepts a remote peer payload, replies with own
//     gossip self-payload.
//   - GET  /health      — liveness check, replies "ok".
type AgentServer struct {
	Identity      *Identity
	Handler       QueryHandler
	AdvertiseHost string

	server   *http.Server
	listener net.Listener

	memory map[string]any
	memMu  sync.Mutex

	// peers is a minimal flat peer table merged from /pop/gossip payloads:
	// enough to answer relay-backs and know seeds. SDKs do not run cosine
	// routing (that stays hub-side, per spec §5).
	peers   map[string]map[string]any
	peersMu sync.Mutex
}

// NewAgentServer binds a listener at host:port (port 0 picks a free port,
// useful in tests) and wires up the /agent/query, /pop/gossip, and /health
// routes. Call Start to begin serving.
func NewAgentServer(identity *Identity, handler QueryHandler, host string, port int) (*AgentServer, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, fmt.Errorf("emfilter: listen %s:%d: %w", host, port, err)
	}
	s := &AgentServer{
		Identity:      identity,
		Handler:       handler,
		AdvertiseHost: host,
		listener:      ln,
		memory:        map[string]any{},
		peers:         map[string]map[string]any{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/query", s.handleAgentQuery)
	mux.HandleFunc("/pop/gossip", s.handleGossip)
	mux.HandleFunc("/health", s.handleHealth)
	s.server = &http.Server{Handler: mux}
	return s, nil
}

// Port returns the TCP port the server is bound to (useful when NewAgentServer
// was called with port 0).
func (s *AgentServer) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// PeerCount returns the number of distinct peer ids merged from /pop/gossip.
func (s *AgentServer) PeerCount() int {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()
	return len(s.peers)
}

// Start begins serving in a background goroutine.
func (s *AgentServer) Start() {
	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[em_filter] agent server error: %v", err)
		}
	}()
}

// Stop closes the listener and any open connections immediately.
func (s *AgentServer) Stop() error {
	return s.server.Close()
}

func (s *AgentServer) handleAgentQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad query"})
		return
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad query"})
		return
	}
	query, ok := m["query"].(string)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad query"})
		return
	}

	s.memMu.Lock()
	result, newMemory, herr := s.Handler(query, s.memory)
	if herr == nil {
		s.memory = newMemory
	}
	s.memMu.Unlock()

	if herr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": herr.Error()})
		return
	}

	items := normalizeResults(result)
	ts, signerID, sig := s.Identity.SignResultsV2(query, items)
	writeJSON(w, http.StatusOK, map[string]any{
		"results":   result,
		"ts":        ts,
		"signer_id": signerID,
		"signature": sig,
	})
}

func (s *AgentServer) handleGossip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var peer map[string]any
	if json.Unmarshal(raw, &peer) == nil {
		if id, ok := peer["id"].(string); ok && id != "" {
			s.peersMu.Lock()
			s.peers[id] = peer
			s.peersMu.Unlock()
		}
	}
	writeJSON(w, http.StatusOK, s.Identity.GossipPayload(s.AdvertiseHost, s.Port()))
}

func (s *AgentServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// normalizeResults decodes v (whatever a QueryHandler returned) back through
// JSON so CanonicalResponse sees the same map[string]any shape regardless of
// the handler's concrete Go type. A non-array result normalizes to nil,
// matching the spec's "a non-list items value produces empty bytes".
func normalizeResults(v any) []any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	arr, _ := out.([]any)
	return arr
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

// GossipPusher periodically POSTs the agent's gossip self-payload to each
// seed disco's /pop/gossip, so the disco can TOFU-bind the pubkey, compute
// the routing vector from capabilities, and relay the peer into the mesh.
type GossipPusher struct {
	Identity  *Identity
	Seeds     []string // base origin URLs, e.g. "http://disco.example.com:8080"
	Host      string   // advertised host
	QueryPort int
	Interval  time.Duration

	client   *http.Client
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewGossipPusher builds a pusher with a default 5s interval when interval <= 0.
func NewGossipPusher(identity *Identity, seeds []string, host string, queryPort int, interval time.Duration) *GossipPusher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &GossipPusher{
		Identity:  identity,
		Seeds:     seeds,
		Host:      host,
		QueryPort: queryPort,
		Interval:  interval,
		client:    &http.Client{Timeout: 5 * time.Second},
		stopCh:    make(chan struct{}),
	}
}

// Start pushes once immediately, then every Interval, until Stop is called.
func (p *GossipPusher) Start() {
	go func() {
		p.PushOnce()
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.PushOnce()
			case <-p.stopCh:
				return
			}
		}
	}()
}

// Stop halts the push loop. Safe to call more than once.
func (p *GossipPusher) Stop() {
	p.stopOnce.Do(func() { close(p.stopCh) })
}

// PushOnce POSTs the self-payload to every seed once, logging (not failing)
// on a per-seed error so one unreachable seed does not block the others.
func (p *GossipPusher) PushOnce() {
	payload := p.Identity.GossipPayload(p.Host, p.QueryPort)
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[em_filter] gossip payload marshal failed: %v", err)
		return
	}
	for _, seed := range p.Seeds {
		url := strings.TrimRight(seed, "/") + "/pop/gossip"
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			log.Printf("[em_filter] gossip push to %s failed: %v", seed, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range p.Identity.GossipHeaders(b) {
			req.Header.Set(k, v)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			log.Printf("[em_filter] gossip push to %s failed: %v", seed, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
