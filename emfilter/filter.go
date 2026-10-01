// Package emfilter is the Go SDK for Emergence filter agents.
//
// Implement the Filter interface and run it with FilterRunner:
//
//	type MyFilter struct{}
//
//	func (f MyFilter) Handle(body string, memory map[string]any) (any, map[string]any, error) {
//	    result := []map[string]any{{"type": "url", "properties": map[string]any{
//	        "url": "https://example.com", "title": "Echo: " + body,
//	    }}}
//	    return result, memory, nil
//	}
//
//	func (f MyFilter) Capabilities() []string { return []string{"search", "query"} }
//
//	func main() { emfilter.NewFilterRunner("my_filter", MyFilter{}, nil).Run() }
package emfilter

// Filter is the handler contract for an Emergence filter agent.
//
// Mirrors the Erlang em_filter handler:
//
//	handle(Body, Memory) -> {Result, NewMemory}
//
// Handle is called for every query the agent receives — a POST to
// /agent/query under Model A (direct), or a WS "query" frame relayed by a
// disco under Model B (relay, the default). memory starts as an empty map
// and persists across queries within a connection/server lifetime. On
// reconnect (Model B) the memory is reset to an empty map (same as Erlang
// ram mode).
type Filter interface {
	// Handle processes an incoming query.
	// body is the raw query string.
	// memory is the current memory state (empty map on first call).
	// Returns the result (JSON-serialisable, typically a []map[string]any of
	// embryo items), the new memory state, and any error. The result is
	// signed with the agent's ed25519 key (see crypto.go) before being sent
	// back as {"results":..., "ts":..., "signer_id":..., "signature":...}.
	// On error, the SDK replies with a 500 (Model A) or an empty signed
	// result (Model B) and continues.
	Handle(body string, memory map[string]any) (result any, newMemory map[string]any, err error)

	// Capabilities returns the list of capabilities this agent advertises
	// (plain strings only — the disco computes the routing vector from
	// them; see spec §5). Advertised in the Model B "hello" frame and the
	// Model A gossip payload. Default implementations should return at
	// least ["search", "query"].
	Capabilities() []string
}
