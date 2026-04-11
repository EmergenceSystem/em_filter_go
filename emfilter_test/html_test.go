package emfilter_test

import (
	"testing"

	"em_filter/emfilter"
)

func TestStripScripts(t *testing.T) {
	html := "<p>Hello</p><script>alert(1)</script><p>World</p>"
	result := emfilter.StripScripts(html)
	if contains(result, "script") {
		t.Error("script tag still present")
	}
	if !contains(result, "Hello") {
		t.Error("content removed")
	}
}

func TestStripScriptsMultiline(t *testing.T) {
	html := "<p>A</p><script type=\"text/javascript\">\nvar x=1;\n</script><p>B</p>"
	result := emfilter.StripScripts(html)
	if contains(result, "var x") {
		t.Error("script content still present")
	}
}

func TestGetText(t *testing.T) {
	if emfilter.GetText("<p>Hello <b>world</b></p>") != "Hello world" {
		t.Error("unexpected text")
	}
}

func TestGetTextNoTags(t *testing.T) {
	if emfilter.GetText("plain text") != "plain text" {
		t.Error("plain text modified")
	}
}

func TestExtractElementsByTag(t *testing.T) {
	elems := emfilter.ExtractElements("<div>A</div><div>B</div>", "div")
	if len(elems) != 2 {
		t.Fatalf("want 2, got %d", len(elems))
	}
}

func TestExtractElementsByClass(t *testing.T) {
	elems := emfilter.ExtractElements(`<li class="b_algo">item</li><li>other</li>`, "li.b_algo")
	if len(elems) != 1 || !contains(elems[0], "item") {
		t.Errorf("unexpected: %v", elems)
	}
}

func TestExtractAttributeFound(t *testing.T) {
	v, ok := emfilter.ExtractAttribute(`<a href="/page">link</a>`, "href")
	if !ok || v != "/page" {
		t.Errorf("want /page, got %q %v", v, ok)
	}
}

func TestExtractAttributeNotFound(t *testing.T) {
	_, ok := emfilter.ExtractAttribute("<a>link</a>", "href")
	if ok {
		t.Error("should not find href")
	}
}

func TestDecodeHtmlEntitiesNumeric(t *testing.T) {
	if emfilter.DecodeHTMLEntities("&#233;") != "é" {
		t.Error("numeric entity not decoded")
	}
}

func TestDecodeHtmlEntitiesHex(t *testing.T) {
	if emfilter.DecodeHTMLEntities("&#xE9;") != "é" {
		t.Error("hex entity not decoded")
	}
}

func TestDecodeHtmlEntitiesNamed(t *testing.T) {
	cases := map[string]string{
		"&eacute;": "é", "&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": "\"",
	}
	for input, want := range cases {
		if got := emfilter.DecodeHTMLEntities(input); got != want {
			t.Errorf("DecodeHTMLEntities(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDecodeHtmlEntitiesCombined(t *testing.T) {
	result := emfilter.DecodeHTMLEntities("caf&eacute; &amp; croissant")
	if result != "café & croissant" {
		t.Errorf("unexpected: %q", result)
	}
}

func TestShouldSkipLinkNotHttp(t *testing.T) {
	if !emfilter.ShouldSkipLink("ftp://example.com", nil) {
		t.Error("should skip non-http")
	}
}

func TestShouldSkipLinkExcluded(t *testing.T) {
	if !emfilter.ShouldSkipLink("https://ads.example.com/x", []string{"ads.example.com"}) {
		t.Error("should skip excluded")
	}
}

func TestShouldSkipLinkOk(t *testing.T) {
	if emfilter.ShouldSkipLink("https://example.com", []string{"ads.com"}) {
		t.Error("should not skip")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
