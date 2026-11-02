// Package webapp serves the Telegram Mini App: its embedded static files and its JSON API.
package webapp

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

// dist holds the Vite build of web/. The committed dist/.gitkeep lets Go build without it, and
// NewStatic then refuses to start.
//
//go:embed all:dist
var dist embed.FS

// ErrNotBuilt means the binary was built without the Mini App.
var ErrNotBuilt = errors.New(
	"the Mini App is not built: run npm --prefix web ci --ignore-scripts && npm --prefix web run build, then build again",
)

// Built returns the embedded Mini App build.
func Built() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // a constant, valid path: fs.Sub fails only on an invalid one
	}

	return sub
}

// contentSecurityPolicy lets the page load only its own files and Telegram's Mini App script.
// Telegram's script inserts <style> tags, so inline styles are allowed; inline scripts are not. frame-ancestors covers Telegram Web, which shows Mini Apps in an iframe.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' https://telegram.org; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; " +
	"form-action 'none'; frame-ancestors https://web.telegram.org"

// Static serves the Mini App build.
type Static struct {
	files http.Handler
}

// NewStatic serves fsys, which must hold index.html.
func NewStatic(fsys fs.FS) (*Static, error) {
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		return nil, fmt.Errorf("%w (%w)", ErrNotBuilt, err)
	}

	return &Static{files: http.FileServerFS(fsys)}, nil
}

func (s *Static) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")

	// Vite puts a content hash in every asset name, so an asset never changes; index.html names
	// the current assets, so it is checked on every load.
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}

	s.files.ServeHTTP(w, r)
}
