package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandlerReturns200WhenAllChecksPass(t *testing.T) {
	checks := map[string]Check{
		"db": func(ctx context.Context) error { return nil },
	}
	handler := Handler(time.Second, checks)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want ok", body["status"])
	}
}

func TestHandlerReturns503WhenACheckFails(t *testing.T) {
	checks := map[string]Check{
		"db": func(ctx context.Context) error { return errors.New("connection refused") },
	}
	handler := Handler(time.Second, checks)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body["status"] != "unavailable" {
		t.Errorf("status field = %v, want unavailable", body["status"])
	}
	errs, ok := body["errors"].(map[string]any)
	if !ok || errs["db"] != "connection refused" {
		t.Errorf("errors field = %v, want db: connection refused", body["errors"])
	}
}

func TestHandlerWithNoChecksReturns200(t *testing.T) {
	handler := Handler(time.Second, map[string]Check{})
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestHandlerRespectsTimeout(t *testing.T) {
	checks := map[string]Check{
		"slow": func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	handler := Handler(20*time.Millisecond, checks)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503 (timed-out check should fail)", rec.Code)
	}
	if elapsed > 2*time.Second {
		t.Errorf("handler took %v, expected to respect short timeout", elapsed)
	}
}
