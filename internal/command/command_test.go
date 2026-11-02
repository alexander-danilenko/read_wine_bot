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

func TestScreenCommandsOpenTheApp(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		command string
		wantURL string
	}{
		{command.Help, appURL + "?help"},
		{command.Language, appURL + "?settings"},
		{command.List, appURL},
		{command.New, appURL + "?new"},
	}

	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			sent := &sentMessage{}
			h := command.ScreenHandlers(texts, savedLanguage("en"), sent, appURL, slog.New(slog.DiscardHandler))[tc.command]
			h(t.Context(), nil, &models.Update{Message: &models.Message{
				From: &models.User{
					ID: 42,
				},
				Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate},
				Text: "/" + tc.command,
			}})

			if sent.userID != 42 || sent.text == "" {
				t.Fatalf("sent %+v, want a text to user 42", sent)
			}

			if len(sent.rows) != 1 || len(sent.rows[0]) != 1 {
				t.Fatalf("rows = %+v, want one button", sent.rows)
			}

			if b := sent.rows[0][0]; b.WebApp == nil || b.WebApp.URL != tc.wantURL || b.Text == "" {
				t.Errorf("button = %+v, want a web_app button to %s", b, tc.wantURL)
			}
		})
	}
}

func TestScreenCommandUsesSavedLanguage(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	en, uk := &sentMessage{}, &sentMessage{}
	msg := &models.Update{Message: &models.Message{
		From: &models.User{ID: 42, LanguageCode: "en"}, Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate},
	}}
	log := slog.New(slog.DiscardHandler)

	command.ScreenHandlers(texts, savedLanguage(""), en, appURL, log)[command.Help](t.Context(), nil, msg)
	command.ScreenHandlers(texts, savedLanguage("uk"), uk, appURL, log)[command.Help](t.Context(), nil, msg)

	if en.text == uk.text || en.rows[0][0].Text == uk.rows[0][0].Text {
		t.Errorf("en %q and uk %q replies match, want the saved uk to win over the app's en", en.text, uk.text)
	}
}

func TestScreenCommandIgnoresGroups(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	sent := &sentMessage{}
	h := command.ScreenHandlers(texts, savedLanguage(""), sent, appURL, slog.New(slog.DiscardHandler))[command.New]
	h(t.Context(), nil, &models.Update{Message: &models.Message{
		From: &models.User{ID: 42}, Chat: models.Chat{ID: -5, Type: models.ChatTypeGroup}, Text: "/new",
	}})

	if sent.userID != 0 {
		t.Errorf("sent %+v in a group, want nothing", sent)
	}
}

// commandsAPI is a Bot API server that answers setMyCommands and keeps the command names it
// got for each language_code.
type commandsAPI struct {
	t   *testing.T
	got map[string][]string
}

func (a *commandsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	//nolint:gosec // G120: the body is capped by MaxBytesReader above
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	var commands []models.BotCommand
	if err := json.Unmarshal([]byte(r.FormValue("commands")), &commands); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	names := make([]string, 0, len(commands))
	for _, c := range commands {
		names = append(names, c.Command)
	}

	a.got[r.FormValue("language_code")] = names

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write([]byte(`{"ok":true,"result":true}`)); err != nil {
		a.t.Error(err)
	}
}

func TestRegisterListsEveryCommand(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	api := &commandsAPI{t: t, got: map[string][]string{}}
	srv := httptest.NewServer(api)
	defer srv.Close()

	b, err := bot.New("123:test", bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	if err := command.Register(t.Context(), b, texts); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{command.Help, command.List, command.New, command.Language}, ",")
	for _, lang := range append([]string{""}, texts.Langs()...) {
		if got := strings.Join(api.got[lang], ","); got != want {
			t.Errorf("commands for %q = %s, want %s", lang, got, want)
		}
	}
}
