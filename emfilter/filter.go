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
// Handle is called for every query frame received from em_disco.
// memory starts as an empty map and persists across queries within a connection.
// On reconnect the memory is reset to an empty map (same as Erlang ram mode).
type Filter interface {
	// Handle processes an incoming query.
	// body is the raw query string.
	// memory is the current memory state (empty map on first call).
	// Returns the result (JSON-serialisable), the new memory state, and any error.
	// On error, the SDK sends null as the result and continues.
	Handle(body string, memory map[string]any) (result any, newMemory map[string]any, err error)

	// Capabilities returns the list of capabilities announced in agent_hello.
	// em_disco uses these to route queries.
	// Default implementations should return at least ["search", "query"].
	Capabilities() []string
}
