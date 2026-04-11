# em_filter_go

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Go SDK for building [Emergence](https://github.com/EmergenceSystem) network agents.

`em_filter_go` lets any Go process join the Emergence distributed discovery network
as a **filter agent** — a service that receives search queries from the `em_disco`
broker, processes them (web search, DNS lookup, LLM call, database query, …), and
returns structured results.

This library is the Go equivalent of the Erlang `em_filter` library: same WebSocket
protocol, same configuration contract, idiomatic Go API.

---

## How it works

```
 ┌─────────────┐    WebSocket     ┌───────────────┐    WebSocket     ┌─────────────┐
 │  em_disco   │ ◄─────────────── │ FilterRunner  │ ───────────────► │  em_disco   │
 │  (broker)   │  query / result  │ (your agent)  │  (multi-node)    │  (replica)  │
 └─────────────┘                  └───────────────┘                  └─────────────┘
                                         │
                                  goroutine per node
                                         │
                                  ┌──────┴──────┐
                                  │ your Filter │
                                  │    impl     │
                                  └─────────────┘
```

1. `FilterRunner.Run()` resolves disco nodes and starts one goroutine per node.
2. Each goroutine maintains a persistent WebSocket connection with automatic reconnection.
3. On a `query` frame, the goroutine calls `Filter.Handle()` and sends back a `result` frame.

---

## Requirements

Go 1.21+. The only external dependency is [`gorilla/websocket`](https://github.com/gorilla/websocket):

```bash
go get github.com/gorilla/websocket@v1.5.1
```

---

## Quick start

```go
package main

import "em_filter/emfilter"

type MyFilter struct{}

func (f MyFilter) Handle(body string, memory map[string]any) (any, map[string]any, error) {
    result := []map[string]any{{
        "type": "url",
        "properties": map[string]any{
            "url":   "https://example.com",
            "title": "Result for: " + body,
        },
    }}
    return result, memory, nil
}

func (f MyFilter) Capabilities() []string { return []string{"search", "query"} }

func main() {
    emfilter.NewFilterRunner("my_filter", MyFilter{}, nil).Run()
}
```

By default the agent connects to `localhost:8080`. Override via environment
variables or `AgentConfig` — see [Configuration](#configuration).

---

## Try the built-in example

```bash
go run examples/echo_filter/main.go

# With a custom broker:
EM_DISCO_HOST=disco.example.com \
EM_DISCO_PORT=443 \
EM_FILTER_JWT_TOKEN=eyJ... \
go run examples/echo_filter/main.go
```

---

## The Filter interface

```go
type Filter interface {
    Handle(body string, memory map[string]any) (result any, newMemory map[string]any, err error)
    Capabilities() []string
}
```

`Handle` is called for every `query` frame from `em_disco`.
- `body` — raw query string (e.g. `"erlang otp"`)
- `memory` — persists between queries within a connection; reset to an empty map on reconnect
  (same as Erlang `em_filter` RAM mode)

On error, the SDK sends `null` as the result and continues.

### Result format

`result` is any JSON-serialisable value — typically a `[]map[string]any` of embryo objects:

| Type | Required properties |
|------|---------------------|
| `"url"` | `url`, `title` |
| `"dns"` | `domain`, `ips` |
| `"text"` | `content` |

`nil` or an empty slice means "no results for this query".

### Capabilities

`Capabilities()` returns the list of capabilities your agent advertises.
`em_disco` uses these to route queries. Default: `["search", "query"]`.

---

## Configuration

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `EM_DISCO_HOST` | — | Disco broker hostname |
| `EM_DISCO_PORT` | — | Disco broker port |
| `EM_FILTER_JWT_TOKEN` | — | JWT for authenticated brokers |
| `EM_FILTER_RECONNECT_MS` | `5000` | Reconnect delay in milliseconds |

### Node resolution order

1. `AgentConfig.DiscoNodes` — explicit list (highest priority)
2. `EM_DISCO_HOST` / `EM_DISCO_PORT` env vars
3. `[em_disco] nodes = …` in `emergence.conf`
4. `localhost:8080` — built-in default

### TLS inference

| Host | Port | Transport |
|------|------|-----------|
| `localhost`, `127.0.0.1`, `::1` | any | `ws://` (plain) |
| any other | 443 | `wss://` (TLS) |
| any other | other | `ws://` (plain) |

### `emergence.conf`

```ini
[em_disco]
nodes = localhost:8080, disco.example.com, [::1]:9000
```

Platform paths:
- **Linux / macOS:** `~/.config/emergence/emergence.conf`
- **Windows:** `%APPDATA%\emergence\emergence.conf`

### Programmatic configuration

```go
config := &emfilter.AgentConfig{
    JWTToken: "eyJ...",
    DiscoNodes: []emfilter.DiscoNode{
        {Host: "disco.example.com",  Port: 443, TLS: true},
        {Host: "disco2.example.com", Port: 443, TLS: true},
    },
}
emfilter.NewFilterRunner("my_filter", MyFilter{}, config).Run()
```

---

## Multi-node

`FilterRunner.Run()` starts one goroutine per disco node and blocks until all exit
(never in normal operation — each goroutine reconnects forever). The `Filter` is
shared across goroutines in read-only fashion — `Handle` must be goroutine-safe
(or stateless, as in the example above). Memory is local to each goroutine.

---

## HTML utilities

```go
import "em_filter/emfilter"

html    := fetchPage(url)
clean   := emfilter.StripScripts(html)                              // remove <script>…</script>
text    := emfilter.GetText(clean)                                  // strip all tags → plain text
links   := emfilter.ExtractElements(html, "li.b_algo")             // CSS selector
href, ok := emfilter.ExtractAttribute(`<a href="/page">`, "href")  // → "/page", true
decoded := emfilter.DecodeHTMLEntities("caf&eacute; &amp; croissant") // → "café & croissant"
skip    := emfilter.ShouldSkipLink("https://ads.example.com", []string{"ads.example.com"})
```

---

## WebSocket protocol

The agent speaks a minimal JSON-over-WebSocket protocol to `em_disco`.

**Agent → Disco:**
```json
{ "action": "register",    "name": "<agent_name>" }
{ "action": "agent_hello", "capabilities": ["search", "query"] }
{ "action": "result",      "id": "<query_id>", "data": <result> }
```

**Disco → Agent:**
```json
{ "action": "query", "id": "<query_id>", "body": "<query_string>" }
```

The library handles the handshake and reconnection automatically.
Your code only implements `Filter.Handle`.

---

## License

[MIT](LICENSE)
