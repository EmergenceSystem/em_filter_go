package emfilter

import (
	"html"
	"regexp"
	"strings"
)

var (
	reScript = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reTags   = regexp.MustCompile(`<[^>]+>`)
)

// StripScripts removes all <script>…</script> blocks from htmlStr.
func StripScripts(htmlStr string) string {
	return reScript.ReplaceAllString(htmlStr, "")
}

// GetText strips all HTML tags, returning plain text.
func GetText(htmlStr string) string {
	return reTags.ReplaceAllString(htmlStr, "")
}

// ExtractElements returns inner HTML of elements matching a simple CSS selector.
// Supported: tag, tag.class, .class, #id, li.b_algo, div a, div p.
func ExtractElements(htmlStr, selector string) []string {
	pat := selectorToPattern(selector)
	if pat == "" {
		return nil
	}
	re := regexp.MustCompile(`(?is)` + pat)
	matches := re.FindAllStringSubmatch(htmlStr, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

// ExtractAttribute extracts the value of attr from an HTML element string.
// Returns the value and true if found, or "" and false otherwise.
func ExtractAttribute(element, attr string) (string, bool) {
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(attr) + `=['"]([^'"]*?)['"]`)
	m := re.FindStringSubmatch(element)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// DecodeHTMLEntities decodes &#N;, &#xHH;, and &name; HTML entities.
// Uses Go's standard html.UnescapeString plus the named entities from em_filter Erlang.
func DecodeHTMLEntities(text string) string {
	// Go's html.UnescapeString handles most named entities and numeric/hex.
	return html.UnescapeString(text)
}

// ShouldSkipLink returns true if url should be skipped
// (not starting with "http" or matching any entry in excluded).
func ShouldSkipLink(urlStr string, excluded []string) bool {
	if !strings.HasPrefix(urlStr, "http") {
		return true
	}
	for _, excl := range excluded {
		if strings.Contains(urlStr, excl) {
			return true
		}
	}
	return false
}

func selectorToPattern(selector string) string {
	switch selector {
	case "li.b_algo":
		return `<li[^>]*class=['"]b_algo['"][^>]*>(.*?)</li>`
	case "div a":
		return `<a[^>]*>(.*?)</a>`
	case "div p":
		return `<p[^>]*>(.*?)</p>`
	}
	if strings.HasPrefix(selector, ".") {
		cls := regexp.QuoteMeta(selector[1:])
		return `<[^>]*class=['"][^'"]*` + cls + `[^'"]*['"][^>]*>(.*?)</[^>]+>`
	}
	if strings.HasPrefix(selector, "#") {
		id := regexp.QuoteMeta(selector[1:])
		return `<[^>]*id=['"]` + id + `['"][^>]*>(.*?)</[^>]+>`
	}
	if i := strings.Index(selector, "."); i >= 0 {
		tag := regexp.QuoteMeta(selector[:i])
		cls := regexp.QuoteMeta(selector[i+1:])
		return `<` + tag + `[^>]*class=['"][^'"]*` + cls + `[^'"]*['"][^>]*>(.*?)</` + tag + `>`
	}
	tag := regexp.QuoteMeta(selector)
	return `<` + tag + `[^>]*>(.*?)</` + tag + `>`
}
