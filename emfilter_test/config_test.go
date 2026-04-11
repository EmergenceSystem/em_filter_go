package emfilter_test

import (
	"testing"

	"em_filter/emfilter"
)

func TestDefaultResolvesToLocalhost(t *testing.T) {
	t.Setenv("EM_DISCO_HOST", "")
	t.Setenv("EM_DISCO_PORT", "")
	nodes := (&emfilter.AgentConfig{}).ResolveNodes()
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Host != "localhost" || n.Port != 8080 || n.TLS {
		t.Errorf("unexpected default: %+v", n)
	}
}

func TestEnvHostAndPort(t *testing.T) {
	t.Setenv("EM_DISCO_HOST", "disco.example.com")
	t.Setenv("EM_DISCO_PORT", "443")
	nodes := (&emfilter.AgentConfig{}).ResolveNodes()
	if nodes[0].Host != "disco.example.com" || nodes[0].Port != 443 || !nodes[0].TLS {
		t.Errorf("unexpected: %+v", nodes[0])
	}
}

func TestEnvHostOnlyRemote(t *testing.T) {
	t.Setenv("EM_DISCO_HOST", "disco.example.com")
	t.Setenv("EM_DISCO_PORT", "")
	nodes := (&emfilter.AgentConfig{}).ResolveNodes()
	if nodes[0].Port != 443 || !nodes[0].TLS {
		t.Errorf("unexpected: %+v", nodes[0])
	}
}

func TestEnvHostOnlyLocalhost(t *testing.T) {
	t.Setenv("EM_DISCO_HOST", "localhost")
	t.Setenv("EM_DISCO_PORT", "")
	nodes := (&emfilter.AgentConfig{}).ResolveNodes()
	if nodes[0].Port != 8080 || nodes[0].TLS {
		t.Errorf("unexpected: %+v", nodes[0])
	}
}

func TestExplicitNodesOverrideEnv(t *testing.T) {
	t.Setenv("EM_DISCO_HOST", "should-be-ignored.com")
	cfg := &emfilter.AgentConfig{
		DiscoNodes: []emfilter.DiscoNode{{Host: "myhost.com", Port: 9000}},
	}
	nodes := cfg.ResolveNodes()
	if len(nodes) != 1 || nodes[0].Host != "myhost.com" {
		t.Errorf("unexpected: %+v", nodes)
	}
}

func TestLocalhostNeverTLS(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		t.Setenv("EM_DISCO_HOST", host)
		t.Setenv("EM_DISCO_PORT", "443")
		nodes := (&emfilter.AgentConfig{}).ResolveNodes()
		if nodes[0].TLS {
			t.Errorf("host %s port 443 should not be TLS", host)
		}
	}
}

func TestRemote443IsTLS(t *testing.T) {
	t.Setenv("EM_DISCO_HOST", "example.com")
	t.Setenv("EM_DISCO_PORT", "443")
	nodes := (&emfilter.AgentConfig{}).ResolveNodes()
	if !nodes[0].TLS {
		t.Error("remote port 443 should be TLS")
	}
}

func TestRemoteOtherPortNoTLS(t *testing.T) {
	t.Setenv("EM_DISCO_HOST", "example.com")
	t.Setenv("EM_DISCO_PORT", "8080")
	nodes := (&emfilter.AgentConfig{}).ResolveNodes()
	if nodes[0].TLS {
		t.Error("remote port 8080 should not be TLS")
	}
}

func TestResolveJWTFromStruct(t *testing.T) {
	t.Setenv("EM_FILTER_JWT_TOKEN", "")
	cfg := &emfilter.AgentConfig{JWTToken: "mytoken"}
	if cfg.ResolveJWT() != "mytoken" {
		t.Error("expected jwt from struct")
	}
}

func TestResolveJWTFromEnv(t *testing.T) {
	t.Setenv("EM_FILTER_JWT_TOKEN", "envtoken")
	cfg := &emfilter.AgentConfig{}
	if cfg.ResolveJWT() != "envtoken" {
		t.Error("expected jwt from env")
	}
}

func TestParseConfSingleNode(t *testing.T) {
	nodes := emfilter.ParseConfForTest("[em_disco]\nnodes = localhost:8080\n")
	if len(nodes) != 1 || nodes[0].Host != "localhost" || nodes[0].Port != 8080 {
		t.Errorf("unexpected: %+v", nodes)
	}
}

func TestParseConfTwoNodes(t *testing.T) {
	nodes := emfilter.ParseConfForTest("[em_disco]\nnodes = localhost:8080, disco.example.com\n")
	if len(nodes) != 2 || nodes[1].Host != "disco.example.com" || nodes[1].Port != 443 {
		t.Errorf("unexpected: %+v", nodes)
	}
}

func TestParseConfLastNodesWins(t *testing.T) {
	nodes := emfilter.ParseConfForTest("[em_disco]\nnodes = localhost:8080\nnodes = localhost:9090\n")
	if len(nodes) != 1 || nodes[0].Port != 9090 {
		t.Errorf("unexpected: %+v", nodes)
	}
}

func TestParseConfCommentsIgnored(t *testing.T) {
	nodes := emfilter.ParseConfForTest("; comment\n[em_disco]\n# another\nnodes = localhost:9000\n")
	if len(nodes) != 1 || nodes[0].Port != 9000 {
		t.Errorf("unexpected: %+v", nodes)
	}
}

func TestParseNodesIPv6(t *testing.T) {
	nodes := emfilter.ParseNodesForTest("[::1]:9000")
	if len(nodes) != 1 || nodes[0].Host != "::1" || nodes[0].Port != 9000 || nodes[0].TLS {
		t.Errorf("unexpected: %+v", nodes)
	}
}
