package emfilter

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Identity holds an agent's ed25519 keypair, name, and advertised
// capabilities, and builds the wire payloads both transports sign: the
// Model B WS "hello" handshake, the Model A gossip push, and signed query
// results.
type Identity struct {
	Name         string
	Capabilities []string
	Pubkey       []byte // 32 bytes
	Seed         []byte // 32-byte ed25519 seed
	ID           []byte // 16 bytes: IDOf(Pubkey)
}

// NewIdentity loads (or creates on first run) the ed25519 keypair under
// keyDir and derives the peer id from it.
func NewIdentity(name, keyDir string, capabilities []string) (*Identity, error) {
	pub, seed, err := LoadOrCreate(keyDir)
	if err != nil {
		return nil, err
	}
	return &Identity{
		Name:         name,
		Capabilities: capabilities,
		Pubkey:       pub,
		Seed:         seed,
		ID:           IDOf(pub),
	}, nil
}

// KeyDir resolves the key directory for name: EM_FILTER_KEY_DIR if set, else
// the per-agent default "./empop_key_<name>/", mirroring the Erlang
// per-port fallback.
func KeyDir(name string) string {
	if dir := os.Getenv("EM_FILTER_KEY_DIR"); dir != "" {
		return dir
	}
	return fmt.Sprintf("./empop_key_%s/", name)
}

func (id *Identity) selfSigB64() string {
	sig := Sign(CanonicalIdentity(id.ID, id.Name), id.Seed)
	return base64.StdEncoding.EncodeToString(sig)
}

// PubkeyB64 returns the base64-encoded public key, as sent on the wire.
func (id *Identity) PubkeyB64() string {
	return base64.StdEncoding.EncodeToString(id.Pubkey)
}

// IDB64 returns the base64-encoded peer id (the on-wire signer_id).
func (id *Identity) IDB64() string {
	return base64.StdEncoding.EncodeToString(id.ID)
}

// HelloPayload builds the Model B WS handshake frame:
//
//	{"action":"hello","name":...,"pubkey":b64,"sig":b64(selfsig),"capabilities":[...]}
func (id *Identity) HelloPayload() map[string]any {
	return map[string]any{
		"action":       "hello",
		"name":         id.Name,
		"pubkey":       id.PubkeyB64(),
		"sig":          id.selfSigB64(),
		"capabilities": id.Capabilities,
	}
}

// GossipPayload builds the Model A gossip push self-payload POSTed to
// /pop/gossip on each seed disco.
func (id *Identity) GossipPayload(host string, queryPort int) map[string]any {
	return map[string]any{
		"id":           id.IDB64(),
		"name":         id.Name,
		"host":         host,
		"query_port":   queryPort,
		"pubkey":       id.PubkeyB64(),
		"sig":          id.selfSigB64(),
		"capabilities": id.Capabilities,
		"role":         "filter",
	}
}

// SignResults signs items (a decoded JSON array) and returns the on-wire
// signer_id/signature pair used in both the /agent/query HTTP response and
// the Model B "result" WS frame.
func (id *Identity) SignResults(items []any) (signerID string, signature string) {
	return SignResponse(items, id.Pubkey, id.Seed)
}

// SignResultsV2 signs items bound to the received query and a fresh
// millisecond timestamp (v2 response signing). It returns the ts that MUST
// be emitted alongside signer_id/signature so the verifier can rebuild
// CanonicalResponseV2(query, ts, items).
func (id *Identity) SignResultsV2(query string, items []any) (ts int64, signerID string, signature string) {
	ts = time.Now().UnixMilli()
	signerID, signature = SignResponseV2(query, ts, items, id.Pubkey, id.Seed)
	return ts, signerID, signature
}

// GossipHeaders returns the x-pop-id / x-pop-ts / x-pop-sig headers that
// authenticate an outbound /pop/gossip POST, computed over the exact body.
func (id *Identity) GossipHeaders(body []byte) map[string]string {
	ts := time.Now().UnixMilli()
	return map[string]string{
		"x-pop-id":  id.IDB64(),
		"x-pop-ts":  strconv.FormatInt(ts, 10),
		"x-pop-sig": SignGossip(id.ID, ts, body, id.Seed),
	}
}
