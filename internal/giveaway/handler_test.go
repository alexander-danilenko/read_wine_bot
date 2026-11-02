package giveaway_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/keyboard"
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
	sends int // sendMessage requests so far; each sent message gets the next ID

	failDelete bool // answers every deleteMessage request with an error
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	//nolint:gosec // G120: test server; MaxBytesReader above caps the body
	// A method without parameters, such as getMe, sends no form.
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	form := map[string]string{}
	if r.MultipartForm != nil {
		for key, values := range r.MultipartForm.Value {
			form[key] = values[0]
		}
	}

	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

	f.mu.Lock()
	f.calls = append(f.calls, call{method: method, form: form})

	if method == "sendMessage" {
		f.sends++
	}

	result := fakeResult(method, f.sends)
	failed := f.failDelete && method == "deleteMessage"
	f.mu.Unlock()

	if failed {
		if _, err := io.WriteString(
			w,
			`{"ok":false,"error_code":400,"description":"Bad Request: message can't be deleted"}`,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		return
	}

	if _, err := io.WriteString(w, `{"ok":true,"result":`+result+`}`); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// fakeResult returns the result of a successful request to method; a sent message gets the
// ID sends.
func fakeResult(method string, sends int) string {
	switch method {
	case "answerCallbackQuery", "deleteMessage":
		return "true"
	case "getChatMember":
		// Everyone, the bot included, is a channel admin who may edit and post messages.
		return `{"status":"administrator","user":{"id":1,"is_bot":false,"first_name":"A"},"can_edit_messages":true,"can_post_messages":true}`
	case "getMe":
		return `{"id":123,"is_bot":true,"first_name":"Bot","username":"test_bot"}`
	case "getChat":
		// Every post link resolves to the test channel.
		return `{"id":-1001234567890,"type":"channel"}`
	default:
		return `{"message_id":` + strconv.Itoa(sends) + `,"date":0,"chat":{"id":1,"type":"private"}}`
	}
}

// handlerSetup is a setup with a bot that talks to a fakeAPI.
type handlerSetup struct {
	*setup

	api   *fakeAPI
	bot   *bot.Bot
	kb    *keyboard.Tracker
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

	s := newSetup(t)

	return &handlerSetup{
		setup: s, api: api, bot: b, kb: keyboard.NewTracker(s.users, slog.New(slog.DiscardHandler)), texts: texts,
	}
}

// sent returns the text of every sendMessage and editMessageText request.
func (h *handlerSetup) sent() []string {
	var texts []string

	for _, c := range h.api.calls {
		if c.method == "sendMessage" || c.method == "editMessageText" {
			texts = append(texts, c.form["text"])
		}
	}

	return texts
}

// keyboard returns the inline buttons of the last sendMessage or editMessageText request.
func (h *handlerSetup) keyboard(t *testing.T) [][]models.InlineKeyboardButton {
	t.Helper()

	for _, v := range slices.Backward(h.api.calls) {
		if v.method != "sendMessage" && v.method != "editMessageText" {
			continue
		}

		var markup models.InlineKeyboardMarkup
		if err := json.Unmarshal([]byte(v.form["reply_markup"]), &markup); err != nil {
			t.Fatal(err)
		}

		return markup.InlineKeyboard
	}

	t.Fatal("no sendMessage request")

	return nil
}

// deleted returns the message_id of every deleteMessage request.
func (h *handlerSetup) deleted() []string {
	var ids []string

	for _, c := range h.api.calls {
		if c.method == "deleteMessage" {
			ids = append(ids, c.form["message_id"])
		}
	}

	return ids
}
