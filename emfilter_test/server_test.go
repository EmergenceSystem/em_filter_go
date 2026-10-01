package emfilter_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"em_filter/emfilter"
)

func echoHandler(body string, memory map[string]any) (any, map[string]any, error) {
	items := []map[string]any{
		{"url": "https://x/1", "title": "T", "resume": "R"},
	}
	return items, memory, nil
}

func TestAgentQuerySigned(t *testing.T) {
	dir := t.TempDir()
	ident, err := emfilter.NewIdentity("srv", dir, []string{"search"})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	srv, err := emfilter.NewAgentServer(ident, echoHandler, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("NewAgentServer: %v", err)
	}
	srv.Start()
	defer srv.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/agent/query", srv.Port())
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(`{"query":"hi"}`)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Results   []any  `json:"results"`
		SignerID  string `json:"signer_id"`
		Signature string `json:"signature"`
		Ts        int64  `json:"ts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	signerID, err := base64.StdEncoding.DecodeString(out.SignerID)
	if err != nil {
		t.Fatalf("decode signer_id: %v", err)
	}
	if string(signerID) != string(ident.ID) {
		t.Error("signer_id does not match identity id")
	}
	sig, err := base64.StdEncoding.DecodeString(out.Signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if out.Ts <= 0 || !emfilter.Verify(emfilter.CanonicalResponseV2("hi", out.Ts, out.Results), sig, ident.Pubkey) {
		t.Error("v2 response signature does not verify")
	}
}

func TestAgentQueryBadRequest(t *testing.T) {
	dir := t.TempDir()
	ident, _ := emfilter.NewIdentity("srv", dir, []string{"search"})
	srv, _ := emfilter.NewAgentServer(ident, echoHandler, "127.0.0.1", 0)
	srv.Start()
	defer srv.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/agent/query", srv.Port())
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAgentQueryHandlerError(t *testing.T) {
	dir := t.TempDir()
	ident, _ := emfilter.NewIdentity("srv", dir, []string{"search"})
	failing := func(body string, memory map[string]any) (any, map[string]any, error) {
		return nil, memory, fmt.Errorf("boom")
	}
	srv, _ := emfilter.NewAgentServer(ident, failing, "127.0.0.1", 0)
	srv.Start()
	defer srv.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/agent/query", srv.Port())
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(`{"query":"hi"}`)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestHealthEndpoint(t *testing.T) {
	dir := t.TempDir()
	ident, _ := emfilter.NewIdentity("srv", dir, []string{"search"})
	srv, _ := emfilter.NewAgentServer(ident, echoHandler, "127.0.0.1", 0)
	srv.Start()
	defer srv.Stop()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", srv.Port()))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestPopGossipRepliesOwnPayload(t *testing.T) {
	dir := t.TempDir()
	ident, _ := emfilter.NewIdentity("srv", dir, []string{"search"})
	srv, _ := emfilter.NewAgentServer(ident, echoHandler, "127.0.0.1", 0)
	srv.Start()
	defer srv.Stop()

	body, _ := json.Marshal(map[string]any{"id": "peer-1", "name": "peer"})
	url := fmt.Sprintf("http://127.0.0.1:%d/pop/gossip", srv.Port())
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["role"] != "filter" || payload["id"] != ident.IDB64() {
		t.Errorf("unexpected self-payload: %+v", payload)
	}
	if srv.PeerCount() != 1 {
		t.Errorf("PeerCount() = %d, want 1", srv.PeerCount())
	}
}

func TestGossipPusherPushesSignedSelfPayload(t *testing.T) {
	dir := t.TempDir()
	ident, _ := emfilter.NewIdentity("pusher", dir, []string{"search"})

	captured := make(chan map[string]any, 1)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pop/gossip" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		captured <- payload
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer stub.Close()

	pusher := emfilter.NewGossipPusher(ident, []string{stub.URL}, "1.2.3.4", 9600, time.Hour)
	pusher.Start()
	defer pusher.Stop()

	select {
	case payload := <-captured:
		if payload["role"] != "filter" || payload["query_port"] != float64(9600) {
			t.Errorf("unexpected gossip payload: %+v", payload)
		}
		sig, err := base64.StdEncoding.DecodeString(payload["sig"].(string))
		if err != nil {
			t.Fatalf("decode sig: %v", err)
		}
		if !emfilter.Verify(emfilter.CanonicalIdentity(ident.ID, "pusher"), sig, ident.Pubkey) {
			t.Error("gossip self-signature does not verify")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gossip push did not arrive")
	}
}
