package emfilter_test

import (
	"testing"

	"em_filter/emfilter"
)

func TestModeDefaultsToRelay(t *testing.T) {
	t.Setenv("EM_FILTER_MODE", "")
	if got := emfilter.Mode(); got != "relay" {
		t.Errorf("Mode() = %q, want %q", got, "relay")
	}
}

func TestModeDirect(t *testing.T) {
	t.Setenv("EM_FILTER_MODE", "direct")
	if got := emfilter.Mode(); got != "direct" {
		t.Errorf("Mode() = %q, want %q", got, "direct")
	}
}

func TestModeBoth(t *testing.T) {
	t.Setenv("EM_FILTER_MODE", "both")
	if got := emfilter.Mode(); got != "both" {
		t.Errorf("Mode() = %q, want %q", got, "both")
	}
}

func TestModeUnknownFallsBackToRelay(t *testing.T) {
	t.Setenv("EM_FILTER_MODE", "bogus")
	if got := emfilter.Mode(); got != "relay" {
		t.Errorf("Mode() = %q, want %q", got, "relay")
	}
}

func TestQueryPortDefault(t *testing.T) {
	t.Setenv("EM_FILTER_QUERY_PORT", "")
	if got := emfilter.QueryPort(); got != 9600 {
		t.Errorf("QueryPort() = %d, want 9600", got)
	}
}

func TestQueryPortFromEnv(t *testing.T) {
	t.Setenv("EM_FILTER_QUERY_PORT", "9601")
	if got := emfilter.QueryPort(); got != 9601 {
		t.Errorf("QueryPort() = %d, want 9601", got)
	}
}

func TestAdvertiseHostDefault(t *testing.T) {
	t.Setenv("EM_FILTER_HOST", "")
	if got := emfilter.AdvertiseHost(); got != "0.0.0.0" {
		t.Errorf("AdvertiseHost() = %q, want %q", got, "0.0.0.0")
	}
}

func TestAdvertiseHostFromEnv(t *testing.T) {
	t.Setenv("EM_FILTER_HOST", "1.2.3.4")
	if got := emfilter.AdvertiseHost(); got != "1.2.3.4" {
		t.Errorf("AdvertiseHost() = %q, want %q", got, "1.2.3.4")
	}
}

func TestRelayURLScheme(t *testing.T) {
	plain := emfilter.RelayURL(emfilter.DiscoNode{Host: "disco.example.com", Port: 8080, TLS: false})
	if plain != "ws://disco.example.com:8080/ws/filter" {
		t.Errorf("RelayURL(plain) = %q", plain)
	}
	tls := emfilter.RelayURL(emfilter.DiscoNode{Host: "disco.example.com", Port: 443, TLS: true})
	if tls != "wss://disco.example.com:443/ws/filter" {
		t.Errorf("RelayURL(tls) = %q", tls)
	}
}

func TestSeedOriginsScheme(t *testing.T) {
	origins := emfilter.SeedOrigins([]emfilter.DiscoNode{
		{Host: "a.example.com", Port: 8080, TLS: false},
		{Host: "b.example.com", Port: 443, TLS: true},
	})
	if len(origins) != 2 || origins[0] != "http://a.example.com:8080" || origins[1] != "https://b.example.com:443" {
		t.Errorf("SeedOrigins() = %+v", origins)
	}
}

// stubFilter is a minimal Filter used only to exercise NewFilterRunner's
// wiring (identity load + mode dispatch selection), not a live loop.
type stubFilter struct{}

func (stubFilter) Handle(body string, memory map[string]any) (any, map[string]any, error) {
	return nil, memory, nil
}
func (stubFilter) Capabilities() []string { return []string{"search"} }

func TestNewFilterRunnerBuildsWithoutError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EM_FILTER_KEY_DIR", dir)
	r := emfilter.NewFilterRunner("wiring_test", stubFilter{}, nil)
	if r == nil {
		t.Fatal("NewFilterRunner returned nil")
	}
}
