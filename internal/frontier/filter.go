package frontier

import (
	"strings"

	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// DomainFilter allows URLs whose host is in (or a subdomain of) an allowed
// domain list. An empty allow-list allows every domain.
type DomainFilter struct {
	allowed map[string]struct{}
}

// NewDomainFilter builds a DomainFilter from a list of allowed domains.
func NewDomainFilter(domains []string) *DomainFilter {
	allowed := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		allowed[d] = struct{}{}
	}
	return &DomainFilter{allowed: allowed}
}

// Allowed reports whether rawURL's host matches, or is a subdomain of, one
// of the configured allowed domains.
func (f *DomainFilter) Allowed(rawURL string) bool {
	if len(f.allowed) == 0 {
		return true
	}
	host, err := urlutil.Domain(rawURL)
	if err != nil {
		return false
	}
	for len(host) > 0 {
		if _, ok := f.allowed[host]; ok {
			return true
		}
		idx := strings.IndexByte(host, '.')
		if idx < 0 {
			break
		}
		host = host[idx+1:]
	}
	return false
}
