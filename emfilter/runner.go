package emfilter

import (
	"log"
	"sync"
)

// FilterRunner starts one goroutine per disco node and blocks until all exit.
//
// Mirrors Erlang's em_filter:start_agent/3 + blocking join.
type FilterRunner struct {
	name   string
	filter Filter
	config *AgentConfig
}

// NewFilterRunner creates a new runner.
// config may be nil (uses all defaults: env vars / emergence.conf / localhost:8080).
func NewFilterRunner(name string, filter Filter, config *AgentConfig) *FilterRunner {
	if config == nil {
		config = &AgentConfig{}
	}
	return &FilterRunner{name: name, filter: filter, config: config}
}

// Run starts all connection goroutines and blocks until they all exit
// (never in normal operation — each goroutine reconnects forever).
func (r *FilterRunner) Run() {
	nodes := r.config.ResolveNodes()
	jwt := r.config.ResolveJWT()
	reconnectMs := ReconnectMs()

	log.Printf("[em_filter] Starting agent '%s' on %d node(s)", r.name, len(nodes))

	var wg sync.WaitGroup
	for _, node := range nodes {
		node := node
		wg.Add(1)
		go func() {
			defer wg.Done()
			newConnection(r.name, node, r.filter, jwt, reconnectMs).run()
		}()
	}
	wg.Wait()
}
