package fetcher

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"golang.org/x/net/html/charset"
)

// decompress wraps body according to the Content-Encoding header. It
// returns body unchanged for "" and "identity".
func decompress(body io.Reader, contentEncoding string) (io.Reader, error) {
	switch contentEncoding {
	case "", "identity":
		return body, nil
	case "gzip":
		r, err := gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		return r, nil
	case "br":
		return brotli.NewReader(body), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedEncoding, contentEncoding)
	}
}

// toUTF8 transcodes body to UTF-8 based on the charset declared in
// contentType (or auto-detected via BOM/content sniffing if absent),
// gracefully handling non-UTF8 encodings.
func toUTF8(body []byte, contentType string) ([]byte, error) {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, fmt.Errorf("detecting charset: %w", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("transcoding to utf-8: %w", err)
	}
	return out, nil
}
