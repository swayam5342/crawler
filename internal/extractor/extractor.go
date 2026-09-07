// Package extractor turns a fetched page's body into structured data: the
// links it points to (for the frontier) and page metadata (for storage).
// The Extractor interface is pluggable so a content-specific parser can be
// swapped in without touching the fetcher or crawler packages.
package extractor

// Link is a single discovered outbound link.
type Link struct {
	URL      string
	NoFollow bool // rel="nofollow" on the <a> tag
}

// Result is everything extracted from one page.
type Result struct {
	Title    string
	Links    []Link
	NoIndex  bool // <meta name="robots" content="noindex" ...>
	Language string
}

// Extractor pulls structured data out of a fetched page body.
type Extractor interface {
	// Extract parses body (already decoded to UTF-8) as fetched from
	// baseURL (used to resolve relative links) and returns what was found.
	Extract(baseURL string, body []byte) (Result, error)
}
