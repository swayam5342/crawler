package main

import (
	"bytes"
	"context"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/swayamsb50/web_crawler/internal/fetcher"
)

// pageFetcher adapts *fetcher.Client to crawler.PageFetcher by extracting
// <a href> links from the fetched HTML.
//
// This is a minimal stand-in for the pluggable structured-extraction
// interface (link extraction + optional content parser) that the data
// pipeline phase will introduce; it will be replaced once that interface
// exists.
type pageFetcher struct {
	client *fetcher.Client
}

func (p *pageFetcher) Fetch(ctx context.Context, rawURL string) ([]string, error) {
	res, err := p.client.Fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return extractLinks(res.FinalURL, res.Body), nil
}

// extractLinks parses body as HTML and returns absolute URLs for every
// <a href> found, resolved against base. Malformed HTML is handled
// gracefully: golang.org/x/net/html tolerates it and simply extracts
// whatever it can parse.
func extractLinks(base string, body []byte) []string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil
	}

	var links []string
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return links
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if token.Data != "a" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key != "href" {
					continue
				}
				href := strings.TrimSpace(attr.Val)
				if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
					continue
				}
				resolved, err := baseURL.Parse(href)
				if err != nil {
					continue
				}
				if resolved.Scheme == "http" || resolved.Scheme == "https" {
					links = append(links, resolved.String())
				}
			}
		}
	}
}
