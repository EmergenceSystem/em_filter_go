package emfilter_test

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"em_filter/emfilter"
)

func TestHelloAndGossipPayload(t *testing.T) {
	dir := t.TempDir()
	ident, err := emfilter.NewIdentity("t", dir, []string{"search"})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}

	h := ident.HelloPayload()
	if h["action"] != "hello" || h["name"] != "t" {
		t.Errorf("unexpected hello payload: %+v", h)
	}
	pub, err := base64.StdEncoding.DecodeString(h["pubkey"].(string))
	if err != nil {
		t.Fatalf("decode pubkey: %v", err)
	}
	if string(pub) != string(ident.Pubkey) {
		t.Error("hello pubkey does not match identity pubkey")
	}

	sig, err := base64.StdEncoding.DecodeString(h["sig"].(string))
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if !emfilter.Verify(emfilter.CanonicalIdentity(ident.ID, "t"), sig, ident.Pubkey) {
		t.Error("hello self-signature does not verify")
	}

	g := ident.GossipPayload("1.2.3.4", 9600)
	if g["query_port"] != 9600 || g["role"] != "filter" {
		t.Errorf("unexpected gossip payload: %+v", g)
	}
	if g["host"] != "1.2.3.4" {
		t.Errorf("unexpected gossip host: %+v", g["host"])
	}
}

func TestKeyDirDefaultsPerAgent(t *testing.T) {
	t.Setenv("EM_FILTER_KEY_DIR", "")
	if got, want := emfilter.KeyDir("myagent"), "./empop_key_myagent/"; got != want {
		t.Errorf("KeyDir = %q, want %q", got, want)
	}
}

func TestKeyDirFromEnv(t *testing.T) {
	t.Setenv("EM_FILTER_KEY_DIR", "/tmp/custom_keys")
	if got, want := emfilter.KeyDir("myagent"), "/tmp/custom_keys"; got != want {
		t.Errorf("KeyDir = %q, want %q", got, want)
	}
}

func TestIdentitySignResultsMatchesFixture(t *testing.T) {
	fx := loadFixture(t)
	dir := t.TempDir()
	// Seed the key dir with the fixture keypair so SignResults reproduces the
	// fixture's response_signature_b64 exactly (ed25519 is deterministic).
	raw := append(mustHex(t, fx.PubkeyHex), mustHex(t, fx.PrivkeyHex)...)
	if err := os.WriteFile(filepath.Join(dir, "node_ed25519.key"), raw, 0o600); err != nil {
		t.Fatalf("write fixture key file: %v", err)
	}

	ident, err := emfilter.NewIdentity(fx.Name, dir, []string{"search"})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	signerID, sig := ident.SignResults(fx.Items)
	if signerID != fx.SignerIDB64 {
		t.Errorf("signer_id mismatch: got %s want %s", signerID, fx.SignerIDB64)
	}
	if sig != fx.ResponseSignatureB64 {
		t.Errorf("signature mismatch: got %s want %s", sig, fx.ResponseSignatureB64)
	}
}

func TestIdentitySignResultsV2BindsQueryAndTs(t *testing.T) {
	ident, err := emfilter.NewIdentity("t", t.TempDir(), []string{"search"})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	items := []any{map[string]any{"url": "https://x/1", "title": "T"}}
	ts, sid, sigB64 := ident.SignResultsV2("hello", items)
	if ts <= 0 {
		t.Fatalf("ts = %d, want > 0", ts)
	}
	if sid != ident.IDB64() {
		t.Error("signer_id does not match identity id")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if !emfilter.Verify(emfilter.CanonicalResponseV2("hello", ts, items), sig, ident.Pubkey) {
		t.Error("v2 signature does not verify over (query, ts, items)")
	}
	if emfilter.Verify(emfilter.CanonicalResponseV2("other", ts, items), sig, ident.Pubkey) {
		t.Error("v2 signature must be bound to the query")
	}
	if emfilter.Verify(emfilter.CanonicalResponseV2("hello", ts+1, items), sig, ident.Pubkey) {
		t.Error("v2 signature must be bound to ts")
	}
}

func TestIdentityGossipHeadersVerify(t *testing.T) {
	ident, err := emfilter.NewIdentity("t", t.TempDir(), []string{"search"})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	body := []byte(`{"id":"x"}`)
	h := ident.GossipHeaders(body)
	if h["x-pop-id"] != ident.IDB64() {
		t.Errorf("x-pop-id = %q, want %q", h["x-pop-id"], ident.IDB64())
	}
	ts, err := strconv.ParseInt(h["x-pop-ts"], 10, 64)
	if err != nil {
		t.Fatalf("x-pop-ts not decimal: %v", err)
	}
	sig, err := base64.StdEncoding.DecodeString(h["x-pop-sig"])
	if err != nil {
		t.Fatalf("decode x-pop-sig: %v", err)
	}
	sum := sha256.Sum256(body)
	if !emfilter.Verify(emfilter.CanonicalGossipAuth(ident.ID, ts, sum[:]), sig, ident.Pubkey) {
		t.Error("gossip auth signature does not verify")
	}
}

func TestGossipPusherSendsSignedHeaders(t *testing.T) {
	ident, err := emfilter.NewIdentity("t", t.TempDir(), []string{"search"})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	type got struct {
		hdr  http.Header
		body []byte
	}
	ch := make(chan got, 1)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- got{r.Header.Clone(), b}
		w.WriteHeader(http.StatusOK)
	}))
	defer stub.Close()

	emfilter.NewGossipPusher(ident, []string{stub.URL}, "127.0.0.1", 1234, 0).PushOnce()

	select {
	case g := <-ch:
		ts, err := strconv.ParseInt(g.hdr.Get("x-pop-ts"), 10, 64)
		if err != nil {
			t.Fatalf("x-pop-ts: %v", err)
		}
		id, err := base64.StdEncoding.DecodeString(g.hdr.Get("x-pop-id"))
		if err != nil || string(id) != string(ident.ID) {
			t.Fatalf("bad x-pop-id: %v", err)
		}
		sig, err := base64.StdEncoding.DecodeString(g.hdr.Get("x-pop-sig"))
		if err != nil {
			t.Fatalf("x-pop-sig: %v", err)
		}
		sum := sha256.Sum256(g.body)
		if !emfilter.Verify(emfilter.CanonicalGossipAuth(id, ts, sum[:]), sig, ident.Pubkey) {
			t.Error("gossip POST signature does not verify over the exact body")
		}
	default:
		t.Fatal("no gossip POST received")
	}
}
