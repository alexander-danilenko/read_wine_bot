package webapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// photoPath is where channel photos are served, outside /api/: an <img> cannot send the
// Authorization header the API needs.
const photoPath = "/photo/"

// fileIDPattern is what a Telegram file ID looks like; anything else is rejected before the
// signature check.
var fileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

// PhotoDownloader downloads a channel photo. giveaway.BotAPI implements it.
type PhotoDownloader interface {
	// ChannelPhoto returns the photo fileID and its content type.
	ChannelPhoto(ctx context.Context, fileID string) ([]byte, string, error)
}

// Photos serves channel photos at URLs the API signs. The page loads them with a plain <img>,
// and the webview caches them for good: a new channel photo gets a new file ID, so a new URL.
// The signature limits the server to the photos the API named, so it never downloads other
// files the bot can reach; the download link, which carries the bot token, stays on the server.
type Photos struct {
	key []byte
	tg  PhotoDownloader
	log *slog.Logger
}

// NewPhotos returns the photo server. Its signing key derives from the bot token, as
// Telegram's initData key does, so it needs no setting of its own.
func NewPhotos(botToken string, tg PhotoDownloader, log *slog.Logger) *Photos {
	key := hmac.New(sha256.New, []byte("ChannelPhoto"))
	key.Write([]byte(botToken))

	return &Photos{key: key.Sum(nil), tg: tg, log: log}
}

// URL returns the signed URL of photo fileID.
func (p *Photos) URL(fileID string) string {
	return photoPath + fileID + "?s=" + p.sign(fileID)
}

func (p *Photos) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fileID := strings.TrimPrefix(r.URL.Path, photoPath)
	if r.Method != http.MethodGet || !fileIDPattern.MatchString(fileID) ||
		!hmac.Equal([]byte(r.URL.Query().Get("s")), []byte(p.sign(fileID))) {
		http.NotFound(w, r)

		return
	}

	data, kind, err := p.tg.ChannelPhoto(r.Context(), fileID)
	if err != nil {
		p.log.WarnContext(r.Context(), "serve channel photo", "error", err)
		http.Error(w, "photo unavailable", http.StatusBadGateway)

		return
	}

	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// ChannelPhoto passes only bytes http.DetectContentType calls an image, which never detects
	// SVG or HTML, and nosniff stops the browser guessing otherwise.
	if _, err := w.Write(data); err != nil { //nolint:gosec // G705: an image, see above
		p.log.DebugContext(r.Context(), "write channel photo", "error", err)
	}
}

func (p *Photos) sign(fileID string) string {
	mac := hmac.New(sha256.New, p.key)
	mac.Write([]byte(fileID))

	return hex.EncodeToString(mac.Sum(nil))
}
