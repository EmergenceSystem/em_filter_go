package emfilter_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
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
