package emfilter_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"em_filter/emfilter"
)

func TestRelayClientHelloQueryResult(t *testing.T) {
	dir := t.TempDir()
	ident, err := emfilter.NewIdentity("relay", dir, []string{"search"})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}

	var upgrader websocket.Upgrader
	resultCh := make(chan map[string]any, 1)

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		var hello map[string]any
		if err := conn.ReadJSON(&hello); err != nil {
			t.Errorf("read hello: %v", err)
			return
		}
		if hello["action"] != "hello" {
			t.Errorf("expected hello frame, got %+v", hello)
			return
		}
		if err := conn.WriteJSON(map[string]any{"action": "hello_ok", "id": hello["pubkey"]}); err != nil {
			t.Errorf("write hello_ok: %v", err)
			return
		}

		if err := conn.WriteJSON(map[string]any{"action": "query", "id": "q1", "body": "hi"}); err != nil {
			t.Errorf("write query: %v", err)
			return
		}

		var result map[string]any
		if err := conn.ReadJSON(&result); err != nil {
			t.Errorf("read result: %v", err)
			return
		}
		resultCh <- result
	}))
	defer stub.Close()

	wsURL := "ws" + strings.TrimPrefix(stub.URL, "http") + "/ws/filter"
	handler := func(body string, memory map[string]any) (any, map[string]any, error) {
		items := []map[string]any{{"url": "https://x/1", "title": "T " + body, "resume": "R"}}
		return items, memory, nil
	}
	client := emfilter.NewRelayClient(ident, handler, wsURL, time.Hour)
	go client.RunForever()
	defer client.Stop()

	select {
	case result := <-resultCh:
		if result["action"] != "result" || result["id"] != "q1" {
			t.Errorf("unexpected result frame: %+v", result)
		}
		sig, err := base64.StdEncoding.DecodeString(result["signature"].(string))
		if err != nil {
			t.Fatalf("decode signature: %v", err)
		}
		signerID, err := base64.StdEncoding.DecodeString(result["signer_id"].(string))
		if err != nil {
			t.Fatalf("decode signer_id: %v", err)
		}
		if string(signerID) != string(ident.ID) {
			t.Error("signer_id does not match identity id")
		}
		items, _ := result["results"].([]any)
		ts, ok := result["ts"].(float64)
		if !ok || ts <= 0 {
			t.Fatalf("missing ts in result frame: %+v", result)
		}
		if !emfilter.Verify(emfilter.CanonicalResponseV2("hi", int64(ts), items), sig, ident.Pubkey) {
			t.Error("result signature does not verify")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no result frame received")
	}
}
