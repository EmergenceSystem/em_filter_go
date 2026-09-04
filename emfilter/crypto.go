package emfilter

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// keyFileName is the on-disk key file name, matching the Erlang em_pop_crypto
// load_or_create/1 layout so a key file is portable across implementations.
const keyFileName = "node_ed25519.key"

// IDOf computes the peer id: the first 16 bytes of SHA-256(pubkey).
func IDOf(pub []byte) []byte {
	h := sha256.Sum256(pub)
	id := make([]byte, 16)
	copy(id, h[:16])
	return id
}

// CanonicalIdentity builds the byte form that is self-signed to prove identity
// ownership: id ‖ 0x00 ‖ name (UTF-8), deliberately excluding host/port so a
// hub may rewrite a leaf's routing fields without breaking the self-signature.
func CanonicalIdentity(id []byte, name string) []byte {
	b := make([]byte, 0, len(id)+1+len(name))
	b = append(b, id...)
	b = append(b, 0)
	b = append(b, []byte(name)...)
	return b
}

// pick returns the UTF-8 bytes of the first string field among keys present
// in p, or nil if none match.
func pick(p map[string]any, keys ...string) []byte {
	for _, k := range keys {
		if s, ok := p[k].(string); ok {
			return []byte(s)
		}
	}
	return nil
}

// CanonicalResponse builds the byte form that is signed to authenticate a
// query response. For each item, in list order:
//   - P is item["properties"] if that is an object, else the item itself.
//   - U = P["url"] if a string, else empty.
//   - T = first string among P["title"], P["label"], else empty.
//   - R = first string among P["resume"], P["value"], P["description"], else empty.
//   - the line is U ‖ 0x00 ‖ T ‖ 0x00 ‖ R ‖ 0x0A.
//
// A non-object item (or a map with no matched fields) yields the empty line
// 0x00 ‖ 0x00 ‖ 0x0A. items must already be a decoded JSON array (e.g. from
// encoding/json); a nil or non-list items value produces empty bytes.
func CanonicalResponse(items []any) []byte {
	var out []byte
	for _, it := range items {
		m, _ := it.(map[string]any)
		p := m
		if sub, ok := m["properties"].(map[string]any); ok {
			p = sub
		}
		out = append(out, pick(p, "url")...)
		out = append(out, 0)
		out = append(out, pick(p, "title", "label")...)
		out = append(out, 0)
		out = append(out, pick(p, "resume", "value", "description")...)
		out = append(out, '\n')
	}
	return out
}

// Sign signs msg with the ed25519 private key derived from the 32-byte seed.
func Sign(msg, seed []byte) []byte {
	return ed25519.Sign(ed25519.NewKeyFromSeed(seed), msg)
}

// Verify reports whether sig is a valid ed25519 signature of msg under pub.
func Verify(msg, sig, pub []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}

// SignResponse signs CanonicalResponse(items) and returns the on-wire
// signer_id (base64 of IDOf(pub)) and signature (base64 of the ed25519 sig),
// matching the {"signer_id":..., "signature":...} shape em_pop expects.
func SignResponse(items []any, pub, seed []byte) (signerID string, signature string) {
	sig := Sign(CanonicalResponse(items), seed)
	return base64.StdEncoding.EncodeToString(IDOf(pub)), base64.StdEncoding.EncodeToString(sig)
}

// LoadOrCreate loads the ed25519 keypair from <keyDir>/node_ed25519.key,
// creating one on first run if absent. The file layout is raw
// pubkey(32 bytes) ‖ seed(32 bytes), matching the Erlang load_or_create/1
// layout, so a key file is portable across implementations.
//
// Go's ed25519.PrivateKey is 64 bytes (seed ‖ pub); the file stores only the
// 32-byte seed, so callers must derive the private key with
// ed25519.NewKeyFromSeed(seed) (see Sign).
func LoadOrCreate(keyDir string) (pub []byte, seed []byte, err error) {
	path := filepath.Join(keyDir, keyFileName)
	raw, err := os.ReadFile(path)
	if err == nil {
		if len(raw) != ed25519.PublicKeySize+ed25519.SeedSize {
			return nil, nil, fmt.Errorf("emfilter: key file %s has unexpected length %d", path, len(raw))
		}
		pub = append([]byte(nil), raw[:ed25519.PublicKeySize]...)
		seed = append([]byte(nil), raw[ed25519.PublicKeySize:ed25519.PublicKeySize+ed25519.SeedSize]...)
		return pub, seed, nil
	}
	if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("emfilter: read key file %s: %w", path, err)
	}

	genPub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("emfilter: generate keypair: %w", err)
	}
	pub = []byte(genPub)
	seed = priv.Seed()

	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("emfilter: create key dir %s: %w", keyDir, err)
	}
	raw = append(append([]byte(nil), pub...), seed...)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, nil, fmt.Errorf("emfilter: write key file %s: %w", path, err)
	}
	return pub, seed, nil
}
