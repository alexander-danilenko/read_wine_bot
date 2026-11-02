package webapp_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

func builtFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html><title>app</title>")},
		"assets/index-a1.js": {Data: []byte("console.log(1)")},
	}
}

func TestStatic(t *testing.T) {
	static, err := webapp.NewStatic(builtFS())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path      string
		wantCode  int
		wantCache string
		wantBody  string
	}{
		{path: "/", wantCode: http.StatusOK, wantCache: "no-cache", wantBody: "<title>app</title>"},
		{path: "/assets/index-a1.js", wantCode: http.StatusOK, wantCache: "immutable", wantBody: "console.log(1)"},
		{path: "/assets/missing.js", wantCode: http.StatusNotFound},
		{path: "/review", wantCode: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			static.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil))

			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tc.wantCode)
			}

			if !strings.Contains(
				rec.Header().Get("Content-Security-Policy"),
				"script-src 'self' https://telegram.org",
			) {
				t.Errorf("CSP = %q", rec.Header().Get("Content-Security-Policy"))
			}

			if tc.wantCode != http.StatusOK {
				return
			}

			if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, tc.wantCache) {
				t.Errorf("Cache-Control = %q, want it to contain %q", got, tc.wantCache)
			}

			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestNewStaticNotBuilt(t *testing.T) {
	_, err := webapp.NewStatic(fstest.MapFS{".gitkeep": {}})
	if !errors.Is(err, webapp.ErrNotBuilt) {
		t.Fatalf("err = %v, want ErrNotBuilt", err)
	}
}

func TestServerRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	srv, err := webapp.Listen(ctx, "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Error(err)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)

	go func() { done <- srv.Run(ctx) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+srv.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}

	cancel()

	if err := <-done; err != nil {
		t.Errorf("Run = %v, want nil after cancel", err)
	}
}

func TestListenBusyPort(t *testing.T) {
	first, err := webapp.Listen(t.Context(), "127.0.0.1:0", http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := webapp.Listen(t.Context(), first.Addr().String(), http.NotFoundHandler()); err == nil {
		t.Fatal("second Listen on a busy port succeeded")
	}
}
