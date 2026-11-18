package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/swayamsb50/web_crawler/internal/control"
	"github.com/swayamsb50/web_crawler/internal/crawler"
	"github.com/swayamsb50/web_crawler/internal/frontier"
)

func newTestHandler(t *testing.T, opts Options) (http.Handler, *frontier.Frontier) {
	t.Helper()
	filter := frontier.NewDomainFilter(nil)
	f := frontier.New(filter, frontier.WithKeepAlive())
	ctl, err := control.New(control.Options{Frontier: f, Filter: filter, Gate: crawler.NewPauseGate(false)})
	if err != nil {
		t.Fatal(err)
	}
	return Handler(ctl, opts), f
}

func do(h http.Handler, method, path, contentType, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPIRequests(t *testing.T) {
	const js = "application/json"
	tests := []struct {
		name       string
		opts       Options
		method     string
		path       string
		ctype      string
		body       string
		hdr        map[string]string
		wantStatus int
		wantBody   string
	}{
		{name: "status ok", method: "GET", path: "/api/status", wantStatus: 200, wantBody: `"state":"idle"`},
		{name: "unknown endpoint", method: "GET", path: "/api/nope", wantStatus: 404},
		{name: "add url", method: "POST", path: "/api/urls", ctype: js, body: `{"urls":["https://example.com/"]}`, wantStatus: 200, wantBody: `"status":"queued"`},
		{name: "add url needs json content type", method: "POST", path: "/api/urls", ctype: "application/x-www-form-urlencoded", body: `urls=x`, wantStatus: 415},
		{name: "add url rejects unknown fields", method: "POST", path: "/api/urls", ctype: js, body: `{"url":"x"}`, wantStatus: 400},
		{name: "add url requires urls", method: "POST", path: "/api/urls", ctype: js, body: `{"urls":[]}`, wantStatus: 400},
		{name: "pause", method: "POST", path: "/api/pause", ctype: js, body: `{}`, wantStatus: 200, wantBody: `"paused"`},
		{name: "reset requires a selection", method: "POST", path: "/api/reset", ctype: js, body: `{}`, wantStatus: 400},
		{name: "reset stats", method: "POST", path: "/api/reset", ctype: js, body: `{"stats":true}`, wantStatus: 200},
		{name: "pages without browser", method: "GET", path: "/api/pages", wantStatus: 501},
		{name: "bad domain", method: "POST", path: "/api/domains", ctype: js, body: `{"domain":"https://x.com/"}`, wantStatus: 400},
		{name: "add domain", method: "POST", path: "/api/domains", ctype: js, body: `{"domain":"x.com"}`, wantStatus: 200, wantBody: `"added":true`},
		{name: "token required", opts: Options{Token: "s3cret"}, method: "GET", path: "/api/status", wantStatus: 401},
		{name: "wrong token", opts: Options{Token: "s3cret"}, method: "GET", path: "/api/status", hdr: map[string]string{"Authorization": "Bearer nope"}, wantStatus: 401},
		{name: "right token", opts: Options{Token: "s3cret"}, method: "GET", path: "/api/status", hdr: map[string]string{"Authorization": "Bearer s3cret"}, wantStatus: 200},
		{name: "cors preflight allowed", opts: Options{AllowedOrigins: []string{"http://localhost:3000"}}, method: "OPTIONS", path: "/api/urls", hdr: map[string]string{"Origin": "http://localhost:3000"}, wantStatus: 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTestHandler(t, tt.opts)
			rec := do(h, tt.method, tt.path, tt.ctype, tt.body, tt.hdr)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body %s does not contain %s", rec.Body.String(), tt.wantBody)
			}
			if !json.Valid(rec.Body.Bytes()) && rec.Code != 204 {
				t.Fatalf("response is not JSON: %s", rec.Body.String())
			}
		})
	}
}

func TestCORSHeadersOnlyForAllowedOrigins(t *testing.T) {
	h, _ := newTestHandler(t, Options{AllowedOrigins: []string{"http://localhost:3000"}})
	rec := do(h, "GET", "/api/status", "", "", map[string]string{"Origin": "http://evil.example"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS header for disallowed origin: %q", got)
	}
	rec = do(h, "GET", "/api/status", "", "", map[string]string{"Origin": "http://localhost:3000"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("CORS header = %q", got)
	}
}
