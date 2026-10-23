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

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
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
	f.mu.Unlock()

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
		// Everyone, the bot included, is a channel admin who may edit posts.
		return `{"status":"administrator","user":{"id":1,"is_bot":false,"first_name":"A"},"can_edit_messages":true}`
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
	return giveaway.MessageHandler(h.svc, h.ed, h.texts, h.users, h.kb, slog.New(slog.DiscardHandler))
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
	giveaway.SetupCallbackHandler(h.svc, h.texts, h.users, h.kb, slog.New(slog.DiscardHandler))(ctx, h.bot, update)

	methods := make([]string, 0, len(h.api.calls))
	for _, c := range h.api.calls {
		methods = append(methods, c.method)
	}

	if strings.Join(methods, ",") != "answerCallbackQuery,deleteMessage" ||
		h.api.calls[0].form["text"] != h.texts.Text("en", i18n.GiveawayOutdated) {
		t.Errorf("calls = %+v; want the outdated popup, then the message deleted", h.api.calls)
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
	for _, step := range []string{
		giveaway.StepEndDate, giveaway.StepEndTime, giveaway.StepWinners, giveaway.StepButtonText,
	} {
		if data := giveaway.SetupCallbackPrefix + "default:" + step; len(data) > 64 {
			t.Errorf("callback data %q is %d bytes, over 64", data, len(data))
		}
	}
}

// callback builds a tap by the admin on a button with data under a private chat message.
func (*handlerSetup) callback(data string) *models.Update {
	return &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:      "q1",
		From:    models.User{ID: adminID, LanguageCode: "en"},
		Data:    data,
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 5, Chat: models.Chat{ID: adminID}}},
	}}
}

func (h *handlerSetup) tap(t *testing.T, data string) {
	t.Helper()

	giveaway.EditCallbackHandler(
		h.ed,
		h.texts,
		h.users,
		h.kb,
		slog.New(slog.DiscardHandler),
	)(
		t.Context(),
		h.bot,
		h.callback(data),
	)
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

// openMenu publishes a giveaway for the admin and forwards its post through the edit button.
func (h *handlerSetup) openMenu(t *testing.T) giveaway.Giveaway {
	t.Helper()

	g := publish(t, h.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"})
	if err := h.ed.BeginEdit(t.Context(), adminID); err != nil {
		t.Fatal(err)
	}

	h.messageHandler()(t.Context(), h.bot, h.message("", true))

	return g
}

func TestEditForwardSendsMenu(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)
	id := strconv.FormatUint(uint64(g.ID), 10)

	rows := h.keyboard(t)

	got := make([]string, 0, len(rows))
	for _, row := range rows {
		for _, button := range row {
			got = append(got, button.CallbackData)
		}
	}

	want := []string{
		"giveaway-edit:field:end_date:" + id,
		"giveaway-edit:field:end_time:" + id,
		"giveaway-edit:field:winners:" + id,
		"giveaway-edit:field:button_text:" + id,
		"giveaway-edit:move:" + id,
		"giveaway-edit:ask_finish:" + id,
		"giveaway-edit:ask:" + id,
		"giveaway-edit:refresh:" + id,
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("menu buttons = %q, want %q", got, want)
	}

	if len(rows[0]) != 2 || rows[0][0].Text != "📝 Date" {
		t.Errorf("first row = %+v, want the end date and end time side by side", rows[0])
	}
}

func TestEditLinkSendsMenu(t *testing.T) {
	for _, link := range []string{"https://t.me/read_wine/42", "https://t.me/c/1234567890/42"} {
		t.Run(link, func(t *testing.T) {
			h := newHandlerSetup(t)

			publish(
				t,
				h.store,
				&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"},
			)

			if err := h.ed.BeginEdit(t.Context(), adminID); err != nil {
				t.Fatal(err)
			}

			h.messageHandler()(t.Context(), h.bot, h.message(link, false))

			if rows := h.keyboard(t); rows[0][0].Text != "📝 Date" {
				t.Errorf("first button = %q, want the field menu", rows[0][0].Text)
			}
		})
	}
}

func TestEditMenuOfCancelled(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	if err := h.store.Cancel(t.Context(), g.ID); err != nil {
		t.Fatal(err)
	}

	h.tap(t, "giveaway-edit:no:"+strconv.FormatUint(uint64(g.ID), 10))

	rows := h.keyboard(t)
	if publish := rows[len(rows)-2][0]; publish.Text != "✅ Publish giveaway" ||
		publish.CallbackData != "giveaway-edit:publish:"+strconv.FormatUint(uint64(g.ID), 10) {
		t.Errorf("button above the last = %+v, want Publish", publish)
	}

	if last := rows[len(rows)-1][0]; last.CallbackData != "giveaway-edit:refresh:"+strconv.FormatUint(
		uint64(g.ID),
		10,
	) {
		t.Errorf("last button = %+v, want Refresh", last)
	}
}

func TestEditRefreshShowsNewParticipants(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	if sent := h.sent(); !strings.Contains(sent[len(sent)-1], "Participants: 0\n") {
		t.Fatalf("menu = %q, want 0 participants", sent[len(sent)-1])
	}

	if _, err := h.store.Join(t.Context(), g.ID, 101); err != nil {
		t.Fatal(err)
	}

	h.api.calls = nil
	h.tap(t, "giveaway-edit:refresh:"+strconv.FormatUint(uint64(g.ID), 10))

	var edited string
	for _, c := range h.api.calls {
		if c.method == "editMessageText" && c.form["message_id"] == "5" {
			edited = c.form["text"]
		}
	}

	if !strings.Contains(edited, "Participants: 1\n") {
		t.Errorf("tapped message edited to %q, want the menu with 1 participant", edited)
	}
}

func TestEditMoveByLink(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	h.tap(t, "giveaway-edit:move:"+strconv.FormatUint(uint64(g.ID), 10))

	rows := h.keyboard(t)
	if len(rows) != 1 || rows[0][0].CallbackData != "giveaway-edit:stop" {
		t.Fatalf("keyboard = %+v, want only Cancel under the move prompt", rows)
	}

	h.messageHandler()(t.Context(), h.bot, h.message("https://t.me/c/1234567890/43", false))

	if got := h.read(t, g.ID); got.MessageID != 43 {
		t.Errorf("post %d, want the giveaway moved to 43", got.MessageID)
	}

	if !slices.Contains(h.sent(), h.texts.Text("en", i18n.EditMoved)) {
		t.Errorf("sent %q, want the moved notice", h.sent())
	}
}

func TestEditTextAtMove(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	h.tap(t, "giveaway-edit:move:"+strconv.FormatUint(uint64(g.ID), 10))
	h.messageHandler()(t.Context(), h.bot, h.message("hello", false))

	if got := h.sent(); got[len(got)-1] != h.texts.Text("en", i18n.EditMovePrompt) {
		t.Errorf("last sent %q, want the move prompt again", got[len(got)-1])
	}

	if h.read(t, g.ID).MessageID != 42 {
		t.Error("giveaway moved on a text message")
	}
}

func TestEditTextAtField(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	h.tap(t, "giveaway-edit:field:winners:"+strconv.FormatUint(uint64(g.ID), 10))

	if sent := h.sent(); sent[len(sent)-1] != h.texts.Text("en", i18n.GiveawayAskWinners) {
		t.Errorf("last message = %q, want the winners question", sent[len(sent)-1])
	}

	h.messageHandler()(t.Context(), h.bot, h.message("3", false))

	if got := h.read(t, g.ID); got.Winners != 3 {
		t.Errorf("winners = %d, want 3", got.Winners)
	}

	if rows := h.keyboard(t); len(rows) != 6 {
		t.Errorf("reply has %d button rows, want the menu's 6", len(rows))
	}
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

func TestEditKeepsOneKeyboard(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t) // the menu is message 1

	// The tap comes from message 5, so the question replaces 5 in place, and the menu, now
	// another message with buttons, goes.
	h.api.calls = nil
	h.tap(t, "giveaway-edit:field:winners:"+strconv.FormatUint(uint64(g.ID), 10))

	if got := strings.Join(h.deleted(), ","); got != "1" {
		t.Errorf("deleted after the tap = %q, want the menu", got)
	}

	edited := false
	for _, c := range h.api.calls {
		edited = edited || c.method == "editMessageText" && c.form["message_id"] == "5"
	}

	if !edited {
		t.Errorf("calls = %+v; want the tapped message edited into the question", h.api.calls)
	}

	h.api.calls = nil
	h.messageHandler()(t.Context(), h.bot, h.message("3", false))

	if got := strings.Join(h.deleted(), ","); got != "5" {
		t.Errorf("deleted after the typed value = %q, want the question", got)
	}
}

func TestEditCancelButtons(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)
	id := strconv.FormatUint(uint64(g.ID), 10)

	h.tap(t, "giveaway-edit:ask:"+id)

	if sent := h.sent(); sent[len(sent)-1] != h.texts.Text("en", i18n.EditAskCancel) {
		t.Errorf("last message = %q, want the confirmation", sent[len(sent)-1])
	}

	h.api.calls = nil
	h.tap(t, "giveaway-edit:yes:"+id)

	var edit *call
	for i := range h.api.calls {
		if h.api.calls[i].method == "editMessageReplyMarkup" {
			edit = &h.api.calls[i]
		}
	}

	if edit == nil || edit.form["chat_id"] != strconv.FormatInt(channelID, 10) || edit.form["reply_markup"] != "" {
		t.Errorf("edit call = %+v, want the channel post with no reply_markup", edit)
	}

	if got := h.read(t, g.ID); got.Published {
		t.Error("giveaway still published after Yes")
	}

	h.wantSilent(t, h.texts.Text("en", i18n.EditCancelled))
}

// wantSilent checks that the message with text was sent without a sound.
func (h *handlerSetup) wantSilent(t *testing.T, text string) {
	t.Helper()

	for _, c := range h.api.calls {
		if c.method == "sendMessage" && c.form["text"] == text {
			if c.form["disable_notification"] != "true" {
				t.Errorf("notice %q sent with a sound", text)
			}

			return
		}
	}

	t.Errorf("notice %q not sent", text)
}

func TestEditStaleDefault(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	h.api.calls = nil
	h.tap(t, "giveaway-edit:default:winners:"+strconv.FormatUint(uint64(g.ID), 10))

	methods := make([]string, 0, len(h.api.calls))
	for _, c := range h.api.calls {
		methods = append(methods, c.method)
	}

	if strings.Join(methods, ",") != "answerCallbackQuery,deleteMessage" ||
		h.api.calls[0].form["text"] != h.texts.Text("en", i18n.GiveawayOutdated) {
		t.Errorf("calls = %+v; want the outdated popup, then the message deleted", h.api.calls)
	}
}

func TestEditCallbackData(t *testing.T) {
	edit := giveaway.EditCallbackPrefix
	for _, other := range []string{giveaway.SetupCallbackPrefix, giveaway.JoinCallback} {
		if strings.HasPrefix(other, edit) || strings.HasPrefix(edit, other) {
			t.Errorf("edit prefix %q and %q overlap", edit, other)
		}
	}

	// Telegram limits callback data to 64 bytes; the longest edit data has the longest
	// field and the largest giveaway ID.
	if data := giveaway.EditCallbackPrefix + "default:button_text:4294967295"; len(data) > 64 {
		t.Errorf("callback data %q is %d bytes, over 64", data, len(data))
	}
}

func TestListHandler(t *testing.T) {
	h := newHandlerSetup(t)
	publish(
		t,
		h.store,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: h.kyivTime(25, 19, 0)},
	)

	giveaway.ListHandler(
		h.ed,
		h.texts,
		h.users,
		slog.New(slog.DiscardHandler),
	)(
		t.Context(),
		h.bot,
		h.message("", false),
	)

	want := "Your active giveaways:\n\n" +
		`- 🟢 <a href="https://t.me/c/1234567890/42">Post</a> [<a href="https://t.me/test_bot?start=edit_1234567890_42">edit</a>] · ` +
		`ends 2026-10-25 19:00 · 👥 0`
	if sent := h.sent(); len(sent) != 1 || sent[0] != want {
		t.Errorf("sent %q, want only %q", sent, want)
	}
}

func TestEditLinkHandler(t *testing.T) {
	tests := []struct {
		text     string
		wantMenu bool
	}{
		{text: "/start edit_1234567890_42", wantMenu: true},
		{text: "/start edit_1234567890_43"},
		{text: "/start edit_999_42"},
		{text: "/start edit_1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			h := newHandlerSetup(t)
			publish(
				t,
				h.store,
				&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"},
			)

			giveaway.EditLinkHandler(h.ed, h.texts, h.users, h.kb, slog.New(slog.DiscardHandler))(
				t.Context(), h.bot, h.message(tt.text, false),
			)

			if tt.wantMenu {
				if rows := h.keyboard(t); rows[0][0].Text != "📝 Date" {
					t.Errorf("first button = %q, want the field menu", rows[0][0].Text)
				}

				return
			}

			want := h.texts.Text("en", i18n.EditNotFound)
			if got := h.sent(); len(got) != 1 || got[0] != want {
				t.Errorf("sent %q, want only %q", got, want)
			}
		})
	}
}
