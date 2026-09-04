package emfilter_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"em_filter/emfilter"
)

// cryptoFixture mirrors fixtures/crypto_vectors.json, generated from the
// Erlang em_pop_crypto reference so every SDK proves byte-identical crypto
// against the same vectors.
type cryptoFixture struct {
	CanonicalIdentityHex string `json:"canonical_identity_hex"`
	CanonicalResponseHex string `json:"canonical_response_hex"`
	IDHex                string `json:"id_hex"`
	Items                []any  `json:"items"`
	Name                 string `json:"name"`
	PrivkeyHex           string `json:"privkey_hex"`
	PubkeyHex            string `json:"pubkey_hex"`
	ResponseSignatureB64 string `json:"response_signature_b64"`
	SelfsigB64           string `json:"selfsig_b64"`
	SignerIDB64          string `json:"signer_id_b64"`
}

func loadFixture(t *testing.T) cryptoFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "fixtures", "crypto_vectors.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx cryptoFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return fx
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func TestIDOfMatchesFixture(t *testing.T) {
	fx := loadFixture(t)
	pub := mustHex(t, fx.PubkeyHex)
	id := emfilter.IDOf(pub)
	if !bytes.Equal(id, mustHex(t, fx.IDHex)) {
		t.Errorf("id_of mismatch: got %x", id)
	}
	if base64.StdEncoding.EncodeToString(id) != fx.SignerIDB64 {
		t.Errorf("signer_id_b64 mismatch: got %s want %s",
			base64.StdEncoding.EncodeToString(id), fx.SignerIDB64)
	}
}

func TestCanonicalIdentityMatchesFixture(t *testing.T) {
	fx := loadFixture(t)
	id := mustHex(t, fx.IDHex)
	got := emfilter.CanonicalIdentity(id, fx.Name)
	if !bytes.Equal(got, mustHex(t, fx.CanonicalIdentityHex)) {
		t.Errorf("canonical_identity mismatch: got %x", got)
	}
}

func TestSelfSigMatchesFixture(t *testing.T) {
	fx := loadFixture(t)
	id := mustHex(t, fx.IDHex)
	seed := mustHex(t, fx.PrivkeyHex)
	sig := emfilter.Sign(emfilter.CanonicalIdentity(id, fx.Name), seed)
	if base64.StdEncoding.EncodeToString(sig) != fx.SelfsigB64 {
		t.Errorf("selfsig mismatch: got %s want %s",
			base64.StdEncoding.EncodeToString(sig), fx.SelfsigB64)
	}
}

func TestCanonicalResponseMatchesFixture(t *testing.T) {
	fx := loadFixture(t)
	got := emfilter.CanonicalResponse(fx.Items)
	if !bytes.Equal(got, mustHex(t, fx.CanonicalResponseHex)) {
		t.Errorf("canonical_response mismatch:\n got  %x\n want %x", got, mustHex(t, fx.CanonicalResponseHex))
	}
}

func TestResponseSignatureMatchesFixtureAndVerifies(t *testing.T) {
	fx := loadFixture(t)
	seed := mustHex(t, fx.PrivkeyHex)
	pub := mustHex(t, fx.PubkeyHex)
	cr := emfilter.CanonicalResponse(fx.Items)
	sig := emfilter.Sign(cr, seed)
	if base64.StdEncoding.EncodeToString(sig) != fx.ResponseSignatureB64 {
		t.Errorf("response_signature mismatch: got %s want %s",
			base64.StdEncoding.EncodeToString(sig), fx.ResponseSignatureB64)
	}
	if !emfilter.Verify(cr, sig, pub) {
		t.Error("response signature does not verify against fixture pubkey")
	}
}

func TestSignResponseHelperMatchesFixture(t *testing.T) {
	fx := loadFixture(t)
	seed := mustHex(t, fx.PrivkeyHex)
	pub := mustHex(t, fx.PubkeyHex)
	signerID, sig := emfilter.SignResponse(fx.Items, pub, seed)
	if signerID != fx.SignerIDB64 {
		t.Errorf("signer_id mismatch: got %s want %s", signerID, fx.SignerIDB64)
	}
	if sig != fx.ResponseSignatureB64 {
		t.Errorf("signature mismatch: got %s want %s", sig, fx.ResponseSignatureB64)
	}
}

func TestLoadOrCreateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	pub1, seed1, err := emfilter.LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("first load_or_create: %v", err)
	}
	if len(pub1) != 32 || len(seed1) != 32 {
		t.Fatalf("unexpected sizes: pub=%d seed=%d", len(pub1), len(seed1))
	}
	pub2, seed2, err := emfilter.LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("second load_or_create: %v", err)
	}
	if !bytes.Equal(pub1, pub2) || !bytes.Equal(seed1, seed2) {
		t.Error("key not stable across reload")
	}
}

func TestLoadOrCreateFixtureLayout(t *testing.T) {
	fx := loadFixture(t)
	dir := t.TempDir()
	raw := append(mustHex(t, fx.PubkeyHex), mustHex(t, fx.PrivkeyHex)...)
	if err := os.WriteFile(filepath.Join(dir, "node_ed25519.key"), raw, 0o600); err != nil {
		t.Fatalf("write fixture key file: %v", err)
	}
	pub, seed, err := emfilter.LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("load_or_create: %v", err)
	}
	if !bytes.Equal(pub, mustHex(t, fx.PubkeyHex)) || !bytes.Equal(seed, mustHex(t, fx.PrivkeyHex)) {
		t.Error("loaded keypair does not match fixture file layout")
	}
}
