package extractor

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// HTMLExtractor extracts links and metadata from HTML pages using
// golang.org/x/net/html, which tolerates malformed markup by design.
type HTMLExtractor struct{}

// NewHTML returns the default HTML Extractor.
func NewHTML() *HTMLExtractor {
	return &HTMLExtractor{}
}

// Extract implements Extractor.
func (HTMLExtractor) Extract(baseURL string, body []byte) (Result, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return Result{}, err
	}

	var result Result
	var pageNoFollow bool
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			return result, nil
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}

		token := tokenizer.Token()
		switch token.Data {
		case "a":
			if link, ok := extractLink(base, token); ok {
				if pageNoFollow {
					link.NoFollow = true
				}
				result.Links = append(result.Links, link)
			}
		case "title":
			if result.Title == "" {
				tokenizer.Next()
				result.Title = strings.TrimSpace(tokenizer.Token().Data)
			}
		case "meta":
			if applyMeta(&result, token) {
				// A page-wide nofollow directive applies regardless of
				// where in the document it appears (normally <head>,
				// before any <a> tags, but be defensive about malformed
				// ordering by also marking links already collected).
				pageNoFollow = true
				for i := range result.Links {
					result.Links[i].NoFollow = true
				}
			}
		case "html":
			if lang := attr(token, "lang"); lang != "" {
				result.Language = lang
			}
		}
	}
}

func extractLink(base *url.URL, token html.Token) (Link, bool) {
	href := strings.TrimSpace(attr(token, "href"))
	if href == "" || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
		return Link{}, false
	}
	resolved, err := base.Parse(href)
	if err != nil {
		return Link{}, false
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return Link{}, false
	}

	noFollow := false
	for rel := range strings.FieldsSeq(attr(token, "rel")) {
		if strings.EqualFold(rel, "nofollow") {
			noFollow = true
			break
		}
	}
	return Link{URL: resolved.String(), NoFollow: noFollow}, true
}

// applyMeta looks for <meta name="robots" content="..."> and records the
// noindex directive on result. It returns true if a page-wide nofollow
// directive was found, so the caller can mark every link on the page.
func applyMeta(result *Result, token html.Token) (pageNoFollow bool) {
	if !strings.EqualFold(attr(token, "name"), "robots") {
		return false
	}
	content := attr(token, "content")
	for directive := range strings.SplitSeq(content, ",") {
		switch strings.ToLower(strings.TrimSpace(directive)) {
		case "noindex":
			result.NoIndex = true
		case "nofollow":
			pageNoFollow = true
		}
	}
	return pageNoFollow
}

func attr(token html.Token, key string) string {
	for _, a := range token.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
