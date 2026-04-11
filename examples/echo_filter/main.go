// echo_filter — minimal Emergence filter agent example in Go.
//
// Usage:
//
//	go run examples/echo_filter/main.go
//
// With a custom broker:
//
//	EM_DISCO_HOST=disco.example.com EM_DISCO_PORT=443 \
//	EM_FILTER_JWT_TOKEN=eyJ... go run examples/echo_filter/main.go
package main

import (
	"log"

	"em_filter/emfilter"
)

type EchoFilter struct{}

func (f EchoFilter) Handle(body string, memory map[string]any) (any, map[string]any, error) {
	log.Printf("[echo_filter] query: %s", body)
	result := []map[string]any{
		{
			"type": "url",
			"properties": map[string]any{
				"url":   "https://example.com",
				"title": "Echo: " + body,
			},
		},
	}
	return result, memory, nil
}

func (f EchoFilter) Capabilities() []string {
	return []string{"search", "query", "echo"}
}

func main() {
	emfilter.NewFilterRunner("echo_filter", EchoFilter{}, nil).Run()
}
