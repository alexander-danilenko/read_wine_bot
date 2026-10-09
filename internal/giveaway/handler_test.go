package giveaway_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// call is one Bot API request the handlers made.
type call struct {
	method string
	form   map[string]string // the request's form fields; the bot sends multipart forms
}

// fakeAPI is a Bot API server that records every request and answers it with success.
type fakeAPI struct {
	mu    sync.Mutex
	calls []call
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	//nolint:gosec // G120: test server; MaxBytesReader above caps the body
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	form := map[string]string{}
	for key, values := range r.MultipartForm.Value {
		form[key] = values[0]
	}

	f.mu.Lock()
	f.calls = append(f.calls, call{method: r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], form: form})
	f.mu.Unlock()

	result := `{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}`
	if strings.HasSuffix(r.URL.Path, "answerCallbackQuery") {
		result = "true"
	}

	if _, err := io.WriteString(w, `{"ok":true,"result":`+result+`}`); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handlerSetup is a setup with a bot that talks to a fakeAPI.
type handlerSetup struct {
	*setup

	api   *fakeAPI
	bot   *bot.Bot
	texts *i18n.Catalog
}

func newHandlerSetup(t *testing.T) *handlerSetup {
	t.Helper()

	api := &fakeAPI{}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	b, err := bot.New("123:test", bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	return &handlerSetup{setup: newSetup(t), api: api, bot: b, texts: texts}
}

// sent returns the text of every sendMessage request.
func (h *handlerSetup) sent() []string {
	var texts []string

	for _, c := range h.api.calls {
		if c.method == "sendMessage" {
			texts = append(texts, c.form["text"])
		}
	}

	return texts
}

func (h *handlerSetup) message(text string, forward bool) *models.Update {
	msg := &models.Message{
		Text: text,
		Chat: models.Chat{ID: adminID, Type: models.ChatTypePrivate},
		From: &models.User{ID: adminID, LanguageCode: "en"},
	}

	if forward {
		msg.ForwardOrigin = &models.MessageOrigin{MessageOriginChannel: &models.MessageOriginChannel{
			Chat: models.Chat{ID: channelID, Title: "Read Wine"}, MessageID: 42,
		}}
	}

	return &models.Update{Message: msg}
}

func (h *handlerSetup) messageHandler() bot.HandlerFunc {
	return giveaway.MessageHandler(h.svc, h.texts, h.users, slog.New(slog.DiscardHandler))
}

func TestMessageHandlerWithoutSetup(t *testing.T) {
	h := newHandlerSetup(t)

	h.messageHandler()(t.Context(), h.bot, h.message("hello", false))

	if len(h.api.calls) != 0 {
		t.Errorf("calls = %+v, want none", h.api.calls)
	}
}

func TestMessageHandlerWrongKindRepeatsPrompt(t *testing.T) {
	tests := []struct {
		name    string
		atPost  bool
		forward bool
		want    i18n.Key
	}{
		{name: "text at the post step", atPost: true, want: i18n.GiveawayPrompt},
		{name: "forward at the date step", forward: true, want: i18n.GiveawayAskEndDate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandlerSetup(t)
			ctx := t.Context()

			if err := h.svc.Begin(ctx, adminID); err != nil {
				t.Fatal(err)
			}

			if !tt.atPost {
				h.svc.Forward(ctx, h.tg, adminID, channelID, 42, "en")
			}

			h.messageHandler()(ctx, h.bot, h.message("hello", tt.forward))

			want := h.texts.Text("en", tt.want)
			if got := h.sent(); len(got) != 1 || got[0] != want {
				t.Errorf("sent %q, want only %q", got, want)
			}
		})
	}
}

func TestMessageHandlerInvalidValue(t *testing.T) {
	h := newHandlerSetup(t)
	ctx := t.Context()

	if err := h.svc.Begin(ctx, adminID); err != nil {
		t.Fatal(err)
	}

	h.svc.Forward(ctx, h.tg, adminID, channelID, 42, "en")
	h.messageHandler()(ctx, h.bot, h.message("25.10.2026", false))

	want := []string{h.texts.Text("en", i18n.GiveawayInvalidDate), h.texts.Text("en", i18n.GiveawayAskEndDate)}
	if got := h.sent(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("sent %q, want %q", got, want)
	}
}

func TestSetupCallbackStaleDefault(t *testing.T) {
	h := newHandlerSetup(t)
	ctx := t.Context()

	h.reachSummary(t)

	update := &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:      "q1",
		From:    models.User{ID: adminID, LanguageCode: "en"},
		Data:    giveaway.SetupCallbackPrefix + "default:" + giveaway.StepEndTime,
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 5, Chat: models.Chat{ID: adminID}}},
	}}
	giveaway.SetupCallbackHandler(h.svc, h.texts, h.users, slog.New(slog.DiscardHandler))(ctx, h.bot, update)

	methods := make([]string, 0, len(h.api.calls))
	for _, c := range h.api.calls {
		methods = append(methods, c.method)
	}

	if strings.Join(methods, ",") != "answerCallbackQuery,editMessageReplyMarkup" ||
		h.api.calls[0].form["text"] != h.texts.Text("en", i18n.GiveawayOutdated) {
		t.Errorf("calls = %+v; want the outdated popup, then the buttons removed", h.api.calls)
	}

	if step := h.step(t); step != giveaway.StepConfirm {
		t.Errorf("step = %q, want it unchanged at confirm", step)
	}
}

func TestSetupCallbackData(t *testing.T) {
	if strings.HasPrefix(giveaway.JoinCallback, giveaway.SetupCallbackPrefix) {
		t.Errorf(
			"join callback %q falls under the setup prefix %q",
			giveaway.JoinCallback,
			giveaway.SetupCallbackPrefix,
		)
	}

	// Telegram limits callback data to 64 bytes.
	for _, step := range []string{giveaway.StepEndTime, giveaway.StepWinners, giveaway.StepButtonText} {
		if data := giveaway.SetupCallbackPrefix + "default:" + step; len(data) > 64 {
			t.Errorf("callback data %q is %d bytes, over 64", data, len(data))
		}
	}
}
