package frontier

import (
	"slices"
	"strings"
	"sync"

	"github.com/swayamsb50/web_crawler/internal/urlutil"
)

// DomainFilter allows URLs whose host is in (or a subdomain of) an allowed
// domain list. An empty allow-list allows every domain. The list can be
// changed at runtime; DomainFilter is safe for concurrent use.
type DomainFilter struct {
	mu      sync.RWMutex
	allowed map[string]struct{}
}

// NewDomainFilter builds a DomainFilter from a list of allowed domains.
func NewDomainFilter(domains []string) *DomainFilter {
	allowed := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		if d = normalizeDomain(d); d != "" {
			allowed[d] = struct{}{}
		}
	}
	return &DomainFilter{allowed: allowed}
}

// Add allows domain (and its subdomains). Returns false if domain is
// empty or already allowed.
func (f *DomainFilter) Add(domain string) bool {
	domain = normalizeDomain(domain)
	if domain == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.allowed[domain]; ok {
		return false
	}
	f.allowed[domain] = struct{}{}
	return true
}

// Remove stops allowing domain. Removing the last domain empties the
// allow-list, which allows every domain. Returns false if domain was not
// in the list.
func (f *DomainFilter) Remove(domain string) bool {
	domain = normalizeDomain(domain)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.allowed[domain]; !ok {
		return false
	}
	delete(f.allowed, domain)
	return true
}

// Domains returns the allowed domains, sorted.
func (f *DomainFilter) Domains() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]string, 0, len(f.allowed))
	for d := range f.allowed {
		out = append(out, d)
	}
	slices.Sort(out)
	return out
}

func normalizeDomain(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}

// Allowed reports whether rawURL's host matches, or is a subdomain of, one
// of the configured allowed domains.
func (f *DomainFilter) Allowed(rawURL string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
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
