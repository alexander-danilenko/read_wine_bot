package webapp_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

// newTestMux returns the Mini App's routes over fakes, with users' saved languages.
func newTestMux(t *testing.T, ops *webapp.Operations, saved map[int64]string) *http.ServeMux {
	t.Helper()

	static, err := webapp.NewStatic(builtFS())
	if err != nil {
		t.Fatal(err)
	}

	users := &fakeUsers{saved: saved}
	auth := webapp.NewAuth(testToken, users, users, []string{"en", "uk"}, slog.New(slog.DiscardHandler))

	return webapp.NewMux(static, auth, ops, "test")
}

// call sends GET path as u (no Authorization header when u is nil) and returns the response.
func call(t *testing.T, mux http.Handler, path string, u *models.User) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if u != nil {
		req.Header.Set("Authorization", authorize(t, *u))
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	return rec
}

func TestMe(t *testing.T) {
	mux := newTestMux(t, &webapp.Operations{}, map[int64]string{42: "en"})

	rec := call(t, mux, "/api/me", &models.User{ID: 42, FirstName: "Alice"})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	var got struct {
		Language  string `json:"language"`
		FirstName string `json:"first_name"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Language != "en" || got.FirstName != "Alice" {
		t.Errorf("me = %+v, want en, Alice", got)
	}

	if rec := call(t, mux, "/api/me", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("without launch data: code = %d, want 401", rec.Code)
	}
}

// Huma serves its spec, docs and schemas by default; the deployed bot must not.
func TestNoPublicSpecOrDocs(t *testing.T) {
	mux := newTestMux(t, &webapp.Operations{}, nil)
	alice := &models.User{ID: 42}

	for _, path := range []string{
		"/openapi.json", "/openapi.yaml", "/docs", "/schemas/APIError.json",
		"/api/openapi.json", "/api/openapi.yaml", "/api/docs", "/api/schemas/APIError.json",
	} {
		if rec := call(t, mux, path, alice); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: code = %d, want 404", path, rec.Code)
		}
	}
}
