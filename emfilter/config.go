package emfilter

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// DiscoNode is a single em_disco broker node.
type DiscoNode struct {
	Host string
	Port int
	TLS  bool
}

// AgentConfig configures a filter agent.
// All fields are optional; if DiscoNodes is empty, nodes are resolved automatically.
type AgentConfig struct {
	// JWTToken is the JWT token for WebSocket authentication.
	// If empty, EM_FILTER_JWT_TOKEN env var is checked.
	JWTToken string
	// DiscoNodes is an explicit list of disco nodes.
	// If empty, nodes are resolved automatically.
	DiscoNodes []DiscoNode
}

// ResolveNodes returns the list of disco nodes to connect to.
// Priority: DiscoNodes field → EM_DISCO_HOST/PORT → emergence.conf → localhost:8080.
func (c *AgentConfig) ResolveNodes() []DiscoNode {
	if c != nil && len(c.DiscoNodes) > 0 {
		return c.DiscoNodes
	}

	hostEnv := os.Getenv("EM_DISCO_HOST")
	portEnv := os.Getenv("EM_DISCO_PORT")

	switch {
	case hostEnv != "" && portEnv != "":
		port := parsePort(portEnv, 8080)
		return []DiscoNode{{Host: hostEnv, Port: port, TLS: inferTLS(hostEnv, port)}}
	case hostEnv != "":
		port, tls := defaultPortTLS(hostEnv)
		return []DiscoNode{{Host: hostEnv, Port: port, TLS: tls}}
	case portEnv != "":
		return []DiscoNode{{Host: "localhost", Port: parsePort(portEnv, 8080)}}
	}

	if nodes := readConfNodes(); len(nodes) > 0 {
		return nodes
	}
	return []DiscoNode{{Host: "localhost", Port: 8080}}
}

// ResolveJWT returns the JWT token from config or EM_FILTER_JWT_TOKEN env var.
func (c *AgentConfig) ResolveJWT() string {
	if c != nil && c.JWTToken != "" {
		return c.JWTToken
	}
	return os.Getenv("EM_FILTER_JWT_TOKEN")
}

// ReconnectMs returns the reconnect delay in milliseconds (EM_FILTER_RECONNECT_MS, default 5000).
func ReconnectMs() int {
	v := parsePort(os.Getenv("EM_FILTER_RECONNECT_MS"), 5000)
	if v <= 0 {
		return 5000
	}
	return v
}

func inferTLS(host string, port int) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return false
	}
	return port == 443
}

func defaultPortTLS(host string) (int, bool) {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return 8080, false
	}
	return 443, true
}

func parsePort(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func confPath() string {
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		return filepath.Join(appdata, "emergence", "emergence.conf")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Roaming", "emergence", "emergence.conf")
	}
	return filepath.Join(home, ".config", "emergence", "emergence.conf")
}

func readConfNodes() []DiscoNode {
	path := confPath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return ParseConfForTest(func() string {
		var sb strings.Builder
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteByte('\n')
		}
		return sb.String()
	}())
}

// ParseConfForTest parses an INI-style emergence.conf string.
// Exported for testing only.
func ParseConfForTest(content string) []DiscoNode {
	section := ""
	lastNodes := ""
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section == "em_disco" {
			if eq := strings.IndexByte(line, '='); eq >= 0 {
				key := strings.TrimSpace(line[:eq])
				val := strings.TrimSpace(line[eq+1:])
				if key == "nodes" {
					lastNodes = val
				}
			}
		}
	}
	if lastNodes != "" {
		return ParseNodesForTest(lastNodes)
	}
	return nil
}

// ParseNodesForTest parses a comma-separated node list string.
// Exported for testing only.
func ParseNodesForTest(s string) []DiscoNode {
	var nodes []DiscoNode
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// IPv6 bracket notation: [::1]:9000
		if strings.HasPrefix(entry, "[") {
			close := strings.Index(entry, "]")
			if close < 0 {
				continue
			}
			host := entry[1:close]
			rest := entry[close+1:]
			var port int
			var tls bool
			if strings.HasPrefix(rest, ":") {
				port = parsePort(rest[1:], 8080)
				tls = inferTLS(host, port)
			} else {
				port, tls = defaultPortTLS(host)
			}
			nodes = append(nodes, DiscoNode{Host: host, Port: port, TLS: tls})
			continue
		}
		if idx := strings.LastIndex(entry, ":"); idx >= 0 {
			host := entry[:idx]
			port := parsePort(entry[idx+1:], 8080)
			nodes = append(nodes, DiscoNode{Host: host, Port: port, TLS: inferTLS(host, port)})
		} else {
			port, tls := defaultPortTLS(entry)
			nodes = append(nodes, DiscoNode{Host: entry, Port: port, TLS: tls})
		}
	}
	return nodes
}
