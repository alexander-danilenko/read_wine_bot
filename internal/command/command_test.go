package command_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/command"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

const appURL = "https://bot.example/app"

type savedLanguage string

func (s savedLanguage) Language(context.Context, int64) (string, error) { return string(s), nil }

// sentMessage records what StartHandler sent through the keyboard tracker.
type sentMessage struct {
	userID int64
	text   string
	rows   [][]models.InlineKeyboardButton
}

func (s *sentMessage) SendHTML(
	_ context.Context,
	_ *bot.Bot,
	userID int64,
	text string,
	rows [][]models.InlineKeyboardButton,
) {
	s.userID, s.text, s.rows = userID, text, rows
}

func TestStartOpensApp(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	sent := &sentMessage{}
	h := command.StartHandler(texts, savedLanguage("en"), sent, appURL, "9.9.9", slog.New(slog.DiscardHandler))
	h(t.Context(), nil, &models.Update{Message: &models.Message{
		From: &models.User{ID: 42}, Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate}, Text: "/start",
	}})

	if sent.userID != 42 || !strings.Contains(sent.text, "v9.9.9") {
		t.Fatalf("sent %+v, want the greeting with the version to user 42", sent)
	}

	if len(sent.rows) != 1 || len(sent.rows[0]) != 1 {
		t.Fatalf("rows = %+v, want one button", sent.rows)
	}

	if b := sent.rows[0][0]; b.WebApp == nil || b.WebApp.URL != appURL || b.Text != texts.Text("en", i18n.AppOpen) {
		t.Errorf("button = %+v, want a web_app button to %s", b, appURL)
	}
}

func TestStartIgnoresGroups(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	sent := &sentMessage{}
	h := command.StartHandler(texts, savedLanguage(""), sent, appURL, "1", slog.New(slog.DiscardHandler))
	h(t.Context(), nil, &models.Update{Message: &models.Message{
		From: &models.User{ID: 42}, Chat: models.Chat{ID: -5, Type: models.ChatTypeGroup}, Text: "/start",
	}})

	if sent.userID != 0 {
		t.Errorf("sent %+v in a group, want nothing", sent)
	}
}

// menuButtonAPI is a Bot API server that answers setChatMenuButton and keeps its menu_button.
type menuButtonAPI struct {
	t   *testing.T
	got map[string]any
}

func (a *menuButtonAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	//nolint:gosec // G120: the body is capped by MaxBytesReader above
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	if !strings.HasSuffix(r.URL.Path, "/setChatMenuButton") {
		http.Error(w, "unexpected method", http.StatusNotFound)

		return
	}

	if err := json.Unmarshal([]byte(r.FormValue("menu_button")), &a.got); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write([]byte(`{"ok":true,"result":true}`)); err != nil {
		a.t.Error(err)
	}
}

func TestSetMenuButton(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	api := &menuButtonAPI{t: t}
	srv := httptest.NewServer(api)
	defer srv.Close()

	b, err := bot.New("123:test", bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	if err := command.SetMenuButton(t.Context(), b, texts, appURL); err != nil {
		t.Fatal(err)
	}

	got := api.got
	webApp, ok := got["web_app"].(map[string]any)

	if !ok || got["type"] != "web_app" || webApp["url"] != appURL ||
		got["text"] != texts.Text(i18n.DefaultLang, i18n.AppMenu) {
		t.Errorf("menu_button = %v, want a web_app button to %s", got, appURL)
	}
}
