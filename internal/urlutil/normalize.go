// Package urlutil provides URL normalization used for deduplication and
// scope filtering.
package urlutil

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Normalize canonicalizes a URL so that equivalent URLs map to the same
// string: lower-cased scheme/host, default ports stripped, fragment
// removed, query parameters sorted, and trailing slash on the root path
// normalized.
func Normalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parsing url: %w", err)
	}
	if !u.IsAbs() {
		return "", fmt.Errorf("url %q is not absolute", raw)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Host = stripDefaultPort(u.Scheme, u.Host)
	u.Fragment = ""
	u.RawFragment = ""

	if u.Path == "" {
		u.Path = "/"
	} else if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Path == "" {
			u.Path = "/"
		}
	}

	u.RawQuery = sortedQuery(u.RawQuery)

	return u.String(), nil
}

// Domain returns the lower-cased hostname (no port) for a normalized or raw
// absolute URL.
func Domain(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parsing url: %w", err)
	}
	return strings.ToLower(u.Hostname()), nil
}

func stripDefaultPort(scheme, host string) string {
	switch scheme {
	case "http":
		return strings.TrimSuffix(host, ":80")
	case "https":
		return strings.TrimSuffix(host, ":443")
	default:
		return host
	}
}

func sortedQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Leave malformed queries untouched rather than dropping data.
		return rawQuery
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		vs := values[k]
		sort.Strings(vs)
		for j, v := range vs {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
			_ = i
			_ = j
		}
	}
	return b.String()
}
