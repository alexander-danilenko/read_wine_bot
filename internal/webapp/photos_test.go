package webapp_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

type fakePhotos struct{ asked []string }

func (f *fakePhotos) ChannelPhoto(_ context.Context, fileID string) ([]byte, string, error) {
	f.asked = append(f.asked, fileID)

	return []byte("jpeg"), "image/jpeg", nil
}

// Only a URL the server signed downloads anything; it is cached for good.
func TestPhotosServeOnlySignedURLs(t *testing.T) {
	tg := &fakePhotos{}
	photos := webapp.NewPhotos(testToken, tg, slog.New(slog.DiscardHandler))
	signed := photos.URL("AgACAgIAAx0")

	tests := []struct {
		name   string
		url    string
		status int
	}{
		{name: "signed", url: signed, status: http.StatusOK},
		{name: "no signature", url: "/photo/AgACAgIAAx0", status: http.StatusNotFound},
		{
			name:   "signature of another file",
			url:    "/photo/Other" + signed[len("/photo/AgACAgIAAx0"):],
			status: http.StatusNotFound,
		},
		{name: "not a file ID", url: "/photo/../etc?s=x", status: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			photos.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.url, nil))

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}

	if len(tg.asked) != 1 || tg.asked[0] != "AgACAgIAAx0" {
		t.Errorf("downloaded %v, want only the signed file", tg.asked)
	}

	rec := httptest.NewRecorder()
	photos.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, signed, nil))

	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable", got)
	}
}
