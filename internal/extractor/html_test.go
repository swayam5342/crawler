package extractor

import "testing"

func TestExtractLinks(t *testing.T) {
	html := `<html><body>
		<a href="/a">A</a>
		<a href="https://other.com/b">B</a>
		<a href="#section">anchor only</a>
		<a href="javascript:void(0)">js</a>
		<a href="mailto:x@example.com">mail</a>
		<a href="ftp://example.com/file">ftp</a>
	</body></html>`

	result, err := NewHTML().Extract("https://example.com/page", []byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"https://example.com/a", "https://other.com/b"}
	if len(result.Links) != len(want) {
		t.Fatalf("got %d links, want %d: %+v", len(result.Links), len(want), result.Links)
	}
	for i, l := range result.Links {
		if l.URL != want[i] {
			t.Errorf("link[%d] = %q, want %q", i, l.URL, want[i])
		}
	}
}

func TestExtractRelNofollow(t *testing.T) {
	html := `<a href="/a" rel="nofollow">A</a><a href="/b">B</a>`
	result, err := NewHTML().Extract("https://example.com/", []byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Links) != 2 {
		t.Fatalf("got %d links, want 2", len(result.Links))
	}
	if !result.Links[0].NoFollow {
		t.Errorf("expected link 0 (rel=nofollow) to be NoFollow")
	}
	if result.Links[1].NoFollow {
		t.Errorf("expected link 1 to not be NoFollow")
	}
}

func TestExtractMetaRobotsNoindex(t *testing.T) {
	html := `<html><head><meta name="robots" content="noindex"></head><body><a href="/a">A</a></body></html>`
	result, err := NewHTML().Extract("https://example.com/", []byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.NoIndex {
		t.Errorf("expected NoIndex to be true")
	}
	if len(result.Links) != 1 || result.Links[0].NoFollow {
		t.Errorf("expected the link to still be followed when only noindex is set: %+v", result.Links)
	}
}

func TestExtractMetaRobotsNofollowAppliesToAllLinks(t *testing.T) {
	html := `<html><head><meta name="robots" content="noindex, nofollow"></head><body><a href="/a">A</a><a href="/b">B</a></body></html>`
	result, err := NewHTML().Extract("https://example.com/", []byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.NoIndex {
		t.Errorf("expected NoIndex to be true")
	}
	for i, l := range result.Links {
		if !l.NoFollow {
			t.Errorf("link[%d] = %+v, expected NoFollow=true due to page-wide meta nofollow", i, l)
		}
	}
}

func TestExtractMetaNofollowBeforeLinksInMalformedDoc(t *testing.T) {
	// meta appears after some links (malformed order): all links, including
	// earlier ones, must still end up NoFollow.
	html := `<a href="/a">A</a><meta name="robots" content="nofollow"><a href="/b">B</a>`
	result, err := NewHTML().Extract("https://example.com/", []byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, l := range result.Links {
		if !l.NoFollow {
			t.Errorf("link[%d] = %+v, expected NoFollow=true", i, l)
		}
	}
}

func TestExtractTitle(t *testing.T) {
	html := `<html><head><title>  My Page Title  </title></head><body></body></html>`
	result, err := NewHTML().Extract("https://example.com/", []byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "My Page Title" {
		t.Errorf("Title = %q, want %q", result.Title, "My Page Title")
	}
}

func TestExtractLanguage(t *testing.T) {
	html := `<html lang="en-US"><body></body></html>`
	result, err := NewHTML().Extract("https://example.com/", []byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Language != "en-US" {
		t.Errorf("Language = %q, want en-US", result.Language)
	}
}

func TestExtractHandlesMalformedHTML(t *testing.T) {
	html := `<html><body><a href="/a">unclosed tags <div><p>text`
	result, err := NewHTML().Extract("https://example.com/", []byte(html))
	if err != nil {
		t.Fatalf("expected malformed HTML to be handled gracefully, got error: %v", err)
	}
	if len(result.Links) != 1 || result.Links[0].URL != "https://example.com/a" {
		t.Errorf("expected to still extract the link from malformed HTML, got %+v", result.Links)
	}
}

func TestExtractInvalidBaseURL(t *testing.T) {
	_, err := NewHTML().Extract("://not a url", []byte("<a href=\"/a\">A</a>"))
	if err == nil {
		t.Fatalf("expected error for invalid base URL")
	}
}
