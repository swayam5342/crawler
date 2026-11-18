// Package api exposes the crawl control plane (internal/control) as a
// JSON HTTP API under /api/, consumed by the web dashboard in web/.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/swayamsb50/web_crawler/internal/control"
	"github.com/swayamsb50/web_crawler/internal/deadletter"
	"github.com/swayamsb50/web_crawler/internal/storage"
)

const maxBodyBytes = 1 << 20 // 1 MiB per request body

// Options configures the API handler.
type Options struct {
	// Token, when non-empty, is required as "Authorization: Bearer <token>"
	// on every /api/ request.
	Token string
	// AllowedOrigins lists browser origins allowed to call the API
	// directly (CORS). Leave empty when the dashboard proxies requests.
	AllowedOrigins []string
	// Config is a sanitized, read-only view of the running configuration.
	Config any
	Logger *slog.Logger
}

type server struct {
	ctl  *control.Controller
	opts Options
	log  *slog.Logger
}

// Handler returns the /api/ handler. Mount it at "/api/".
func Handler(ctl *control.Controller, opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &server{ctl: ctl, opts: opts, log: opts.Logger.With("component", "api")}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/queue", s.queue)
	mux.HandleFunc("DELETE /api/queue", s.removeQueued)
	mux.HandleFunc("POST /api/urls", s.addURLs)
	mux.HandleFunc("POST /api/pause", s.pause)
	mux.HandleFunc("POST /api/resume", s.resume)
	mux.HandleFunc("GET /api/pages", s.pages)
	mux.HandleFunc("GET /api/pages/domains", s.pageDomains)
	mux.HandleFunc("GET /api/deadletters", s.deadLetters)
	mux.HandleFunc("POST /api/deadletters/retry", s.retryDeadLetters)
	mux.HandleFunc("POST /api/deadletters/discard", s.discardDeadLetters)
	mux.HandleFunc("POST /api/domains", s.allowDomain)
	mux.HandleFunc("DELETE /api/domains", s.disallowDomain)
	mux.HandleFunc("POST /api/reset", s.reset)
	mux.HandleFunc("GET /api/config", s.config)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "unknown endpoint")
	})

	return s.cors(s.auth(s.csrf(mux)))
}

// ---- middleware ----

func (s *server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.originAllowed(origin) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) originAllowed(origin string) bool {
	for _, o := range s.opts.AllowedOrigins {
		if o == "*" || strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

func (s *server) auth(next http.Handler) http.Handler {
	if s.opts.Token == "" {
		return next
	}
	want := []byte("Bearer " + s.opts.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="crawler"`)
			writeError(w, http.StatusUnauthorized, "missing or invalid API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrf requires a JSON content type on state-changing requests with a
// body. Browsers can't send application/json cross-origin without a CORS
// preflight, so a plain HTML form on another site can't drive the API.
func (s *server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---- handlers ----

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ctl.Status())
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"events": s.ctl.Recent(intParam(r, "limit", 50, 200))})
}

func (s *server) queue(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ctl.Queue(intParam(r, "limit", 100, 1000)))
}

func (s *server) removeQueued(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("url")
	if u == "" {
		writeError(w, http.StatusBadRequest, "url query parameter is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": s.ctl.RemoveQueued(u)})
}

type addURLsRequest struct {
	URLs        []string `json:"urls"`
	AllowDomain bool     `json:"allow_domain"`
	Recrawl     bool     `json:"recrawl"`
}

func (s *server) addURLs(w http.ResponseWriter, r *http.Request) {
	var req addURLsRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.URLs) == 0 {
		writeError(w, http.StatusBadRequest, "urls must contain at least one URL")
		return
	}
	results, err := s.ctl.AddURLs(req.URLs, control.AddOptions{AllowDomain: req.AllowDomain, Recrawl: req.Recrawl})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *server) pause(w http.ResponseWriter, r *http.Request) {
	s.ctl.Pause()
	writeJSON(w, http.StatusOK, map[string]any{"state": s.ctl.State()})
}

func (s *server) resume(w http.ResponseWriter, r *http.Request) {
	s.ctl.Resume()
	writeJSON(w, http.StatusOK, map[string]any{"state": s.ctl.State()})
}

func (s *server) pages(w http.ResponseWriter, r *http.Request) {
	q := storage.PageQuery{
		Search: r.URL.Query().Get("q"),
		Domain: r.URL.Query().Get("domain"),
		Limit:  intParam(r, "limit", 50, 500),
		Offset: intParam(r, "offset", 0, 1<<30),
	}
	list, err := s.ctl.Pages(r.Context(), q)
	if err != nil {
		s.storageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) pageDomains(w http.ResponseWriter, r *http.Request) {
	counts, err := s.ctl.PageDomains(r.Context())
	if err != nil {
		s.storageError(w, err)
		return
	}
	if counts == nil {
		counts = []storage.DomainCount{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": counts})
}

func (s *server) deadLetters(w http.ResponseWriter, r *http.Request) {
	entries, err := s.ctl.DeadLetters()
	if err != nil {
		s.log.Warn("reading dead letters failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not read the dead-letter log")
		return
	}
	limit := intParam(r, "limit", 500, 5000)
	total := len(entries)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	if entries == nil {
		entries = []deadletter.Entry{} // encode as [], not null
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": total})
}

type urlsRequest struct {
	URLs []string `json:"urls"` // empty means all
}

func (s *server) retryDeadLetters(w http.ResponseWriter, r *http.Request) {
	var req urlsRequest
	if !decode(w, r, &req) {
		return
	}
	n, err := s.ctl.RetryDeadLetters(req.URLs)
	if err != nil {
		s.log.Warn("retrying dead letters failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"requeued": n})
}

func (s *server) discardDeadLetters(w http.ResponseWriter, r *http.Request) {
	var req urlsRequest
	if !decode(w, r, &req) {
		return
	}
	n, err := s.ctl.DiscardDeadLetters(req.URLs)
	if err != nil {
		s.log.Warn("discarding dead letters failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"removed": n})
}

type domainRequest struct {
	Domain string `json:"domain"`
}

func (s *server) allowDomain(w http.ResponseWriter, r *http.Request) {
	var req domainRequest
	if !decode(w, r, &req) {
		return
	}
	d := strings.TrimSpace(req.Domain)
	if d == "" || strings.ContainsAny(d, "/: ") {
		writeError(w, http.StatusBadRequest, "domain must be a bare host name, e.g. example.com")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"added": s.ctl.AllowDomain(d)})
}

func (s *server) disallowDomain(w http.ResponseWriter, r *http.Request) {
	d := r.URL.Query().Get("domain")
	if d == "" {
		writeError(w, http.StatusBadRequest, "domain query parameter is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": s.ctl.DisallowDomain(d)})
}

func (s *server) reset(w http.ResponseWriter, r *http.Request) {
	var req control.ResetOptions
	if !decode(w, r, &req) {
		return
	}
	if !req.Queue && !req.DeadLetters && !req.Pages && !req.Stats {
		writeError(w, http.StatusBadRequest, "select at least one of queue, dead_letters, pages, stats")
		return
	}
	res, err := s.ctl.Reset(r.Context(), req)
	if err != nil {
		s.log.Warn("reset partially failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"result": res, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res})
}

func (s *server) config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.opts.Config)
}

func (s *server) storageError(w http.ResponseWriter, err error) {
	if errors.Is(err, control.ErrUnsupported) {
		writeError(w, http.StatusNotImplemented, err.Error())
		return
	}
	s.log.Warn("storage query failed", "error", err)
	writeError(w, http.StatusServiceUnavailable, "storage backend unavailable")
}

// ---- helpers ----

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func intParam(r *http.Request, name string, def, max int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
