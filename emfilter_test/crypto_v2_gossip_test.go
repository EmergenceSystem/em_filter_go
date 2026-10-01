package emfilter_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"em_filter/emfilter"
)

// v2Fixture holds the v2 response-signing and gossip-auth vectors from
// fixtures/crypto_vectors.json.
type v2Fixture struct {
	IDHex                  string `json:"id_hex"`
	Items                  []any  `json:"items"`
	PrivkeyHex             string `json:"privkey_hex"`
	PubkeyHex              string `json:"pubkey_hex"`
	SignerIDB64            string `json:"signer_id_b64"`
	V2Query                string `json:"v2_query"`
	V2Ts                   int64  `json:"v2_ts"`
	CanonicalResponseV2Hex string `json:"canonical_response_v2_hex"`
	ResponseV2SignatureB64 string `json:"response_v2_signature_b64"`
	GossipBodyUTF8         string `json:"gossip_body_utf8"`
	GossipBodySHA256Hex    string `json:"gossip_body_sha256_hex"`
	GossipTs               int64  `json:"gossip_ts"`
	CanonicalGossipAuthHex string `json:"canonical_gossip_auth_hex"`
	GossipSignatureB64     string `json:"gossip_signature_b64"`
}

func loadV2Fixture(t *testing.T) v2Fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "fixtures", "crypto_vectors.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx v2Fixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return fx
}

func v2Hex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func TestCanonicalResponseV2MatchesFixture(t *testing.T) {
	fx := loadV2Fixture(t)
	got := emfilter.CanonicalResponseV2(fx.V2Query, fx.V2Ts, fx.Items)
	want := v2Hex(t, fx.CanonicalResponseV2Hex)
	if !bytes.Equal(got, want) {
		t.Errorf("canonical_response_v2 mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestResponseV2SignatureMatchesFixtureAndVerifies(t *testing.T) {
	fx := loadV2Fixture(t)
	seed := v2Hex(t, fx.PrivkeyHex)
	pub := v2Hex(t, fx.PubkeyHex)
	msg := emfilter.CanonicalResponseV2(fx.V2Query, fx.V2Ts, fx.Items)
	sig := emfilter.Sign(msg, seed)
	if got := base64.StdEncoding.EncodeToString(sig); got != fx.ResponseV2SignatureB64 {
		t.Errorf("response_v2 signature mismatch: got %s want %s", got, fx.ResponseV2SignatureB64)
	}
	if !emfilter.Verify(msg, sig, pub) {
		t.Error("response_v2 signature does not verify against fixture pubkey")
	}
}

func TestSignResponseV2HelperMatchesFixture(t *testing.T) {
	fx := loadV2Fixture(t)
	seed := v2Hex(t, fx.PrivkeyHex)
	pub := v2Hex(t, fx.PubkeyHex)
	signerID, sig := emfilter.SignResponseV2(fx.V2Query, fx.V2Ts, fx.Items, pub, seed)
	if signerID != fx.SignerIDB64 {
		t.Errorf("signer_id mismatch: got %s want %s", signerID, fx.SignerIDB64)
	}
	if sig != fx.ResponseV2SignatureB64 {
		t.Errorf("signature mismatch: got %s want %s", sig, fx.ResponseV2SignatureB64)
	}
}

func TestCanonicalGossipAuthMatchesFixture(t *testing.T) {
	fx := loadV2Fixture(t)
	id := v2Hex(t, fx.IDHex)
	h := sha256.Sum256([]byte(fx.GossipBodyUTF8))
	if hex.EncodeToString(h[:]) != fx.GossipBodySHA256Hex {
		t.Fatalf("gossip body sha256 mismatch: got %x", h)
	}
	got := emfilter.CanonicalGossipAuth(id, fx.GossipTs, h[:])
	want := v2Hex(t, fx.CanonicalGossipAuthHex)
	if !bytes.Equal(got, want) {
		t.Errorf("canonical_gossip_auth mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestSignGossipMatchesFixtureAndVerifies(t *testing.T) {
	fx := loadV2Fixture(t)
	id := v2Hex(t, fx.IDHex)
	seed := v2Hex(t, fx.PrivkeyHex)
	pub := v2Hex(t, fx.PubkeyHex)
	got := emfilter.SignGossip(id, fx.GossipTs, []byte(fx.GossipBodyUTF8), seed)
	if got != fx.GossipSignatureB64 {
		t.Errorf("gossip signature mismatch: got %s want %s", got, fx.GossipSignatureB64)
	}
	sig, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	msg := v2Hex(t, fx.CanonicalGossipAuthHex)
	if !emfilter.Verify(msg, sig, pub) {
		t.Error("gossip signature does not verify against fixture pubkey")
	}
}
