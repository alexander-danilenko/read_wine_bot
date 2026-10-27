package giveaway_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
		"giveaway-edit:ask_finish:" + id,
		"giveaway-edit:ask:" + id,
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("menu buttons = %q, want %q", got, want)
	}

	edit := func(field string) string {
		return `<a href="https://t.me/test_bot?start=field_` + field + "_" + id + `">[✏️]</a>`
	}
	wantText := h.texts.Text("en", i18n.EditStatusPublished) + "\n\n" +
		"<b>📄 Post:</b> https://t.me/c/1234567890/42 " + edit("post") + "\n" +
		"<b>📅 Date:</b> — " + edit("end_date") + "\n" +
		"<b>⏰ Time:</b> — (Kyiv time) " + edit("end_time") + "\n" +
		"<b>🏆 Winners:</b> 1 " + edit("winners") + "\n" +
		"<b>👥 Participants:</b> 0\n" +
		"<b>🔘 Button:</b> Go " + edit("button_text")
	if sent := h.sent(); sent[len(sent)-1] != wantText {
		t.Errorf("menu = %q, want %q", sent[len(sent)-1], wantText)
	}

	if mode := h.api.calls[len(h.api.calls)-1].form["parse_mode"]; mode != "HTML" {
		t.Errorf("parse_mode = %q, want HTML", mode)
	}
}

func TestEditMenuEscapesButtonText(t *testing.T) {
	h := newHandlerSetup(t)
	g := publish(t, h.store, &giveaway.Giveaway{
		ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "<b>Join & win</b>",
	})

	text, _, err := h.ed.Menu(t.Context(), g.ID, "uk", "test_bot")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(text, "&lt;b&gt;Join &amp; win&lt;/b&gt;") {
		t.Errorf("menu = %q, want the button text escaped", text)
	}

	if !strings.Contains(text, ">"+h.texts.Text("uk", i18n.EditLink)+"</a>") {
		t.Errorf("menu = %q, want the Ukrainian edit links", text)
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

			if rows := h.keyboard(t); rows[0][0].CallbackData != "giveaway-edit:ask_finish:1" {
				t.Errorf("first button = %+v, want the field menu", rows[0][0])
			}
		})
	}
}

func TestEditMenuOfCancelled(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)
	id := strconv.FormatUint(uint64(g.ID), 10)

	if err := h.store.Cancel(t.Context(), g.ID); err != nil {
		t.Fatal(err)
	}

	h.tap(t, "giveaway-edit:no:"+id)

	rows := h.keyboard(t)
	if last := rows[len(rows)-1][0]; last.Text != "✅ Publish giveaway" ||
		last.CallbackData != "giveaway-edit:publish:"+id {
		t.Errorf("last button = %+v, want Publish", last)
	}
}

func TestEditMenuShowsNewParticipants(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	if sent := h.sent(); !strings.Contains(sent[len(sent)-1], "<b>👥 Participants:</b> 0\n") {
		t.Fatalf("menu = %q, want 0 participants", sent[len(sent)-1])
	}

	if _, err := h.store.Join(t.Context(), g.ID, 101); err != nil {
		t.Fatal(err)
	}

	h.api.calls = nil
	h.tap(t, "giveaway-edit:no:"+strconv.FormatUint(uint64(g.ID), 10))

	var edited, mode string
	for _, c := range h.api.calls {
		if c.method == "editMessageText" && c.form["message_id"] == "5" {
			edited, mode = c.form["text"], c.form["parse_mode"]
		}
	}

	if mode != "HTML" {
		t.Errorf("parse_mode of the menu edited in place = %q, want HTML", mode)
	}

	if !strings.Contains(edited, "<b>👥 Participants:</b> 1\n") {
		t.Errorf("tapped message edited to %q, want the menu with 1 participant", edited)
	}
}

func TestEditMoveByLink(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	h.tapField(t, "post", g.ID)

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

	h.tapField(t, "post", g.ID)
	h.messageHandler()(t.Context(), h.bot, h.message("hello", false))

	if got := h.sent(); got[len(got)-1] != h.texts.Text("en", i18n.EditMovePrompt) {
		t.Errorf("last sent %q, want the move prompt again", got[len(got)-1])
	}

	if h.read(t, g.ID).MessageID != 42 {
		t.Error("giveaway moved on a text message")
	}
}

// tapField taps the field link of field in the menu of giveaway id.
func (h *handlerSetup) tapField(t *testing.T, field string, id uint) {
	t.Helper()

	u := h.message(giveaway.FieldLinkPrefix+field+"_"+strconv.FormatUint(uint64(id), 10), false)
	u.Message.ID = linkTapID

	giveaway.FieldLinkHandler(h.ed, h.texts, h.users, h.kb, slog.New(slog.DiscardHandler))(t.Context(), h.bot, u)
}

func TestEditTextAtField(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)

	h.tapField(t, giveaway.FieldWinners, g.ID)

	if sent := h.sent(); sent[len(sent)-1] != h.texts.Text("en", i18n.GiveawayAskWinners) {
		t.Errorf("last message = %q, want the winners question", sent[len(sent)-1])
	}

	h.messageHandler()(t.Context(), h.bot, h.message("3", false))

	if got := h.read(t, g.ID); got.Winners != 3 {
		t.Errorf("winners = %d, want 3", got.Winners)
	}

	if rows := h.keyboard(t); len(rows) != 2 {
		t.Errorf("reply has %d button rows, want the menu's 2", len(rows))
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

	// A link tap sends /start, so the tap and the menu go, and the question is a new message,
	// message 2.
	h.api.calls = nil
	h.tapField(t, giveaway.FieldWinners, g.ID)

	if got, want := strings.Join(h.deleted(), ","), strconv.Itoa(linkTapID)+",1"; got != want {
		t.Errorf("deleted after the tap = %q, want %q: the /start message and the menu", got, want)
	}

	h.api.calls = nil
	h.messageHandler()(t.Context(), h.bot, h.message("3", false))

	if got := strings.Join(h.deleted(), ","); got != "2" {
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
		h.kb,
		slog.New(slog.DiscardHandler),
	)(
		t.Context(),
		h.bot,
		h.message("", false),
	)

	want := "Your active giveaways:\n\n" +
		`- 🟢 <a href="https://t.me/c/1234567890/42">Post</a> <a href="https://t.me/test_bot?start=edit_1234567890_42">[✏️]</a> · ` +
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
				if rows := h.keyboard(t); rows[0][0].CallbackData != "giveaway-edit:ask_finish:1" {
					t.Errorf("first button = %+v, want the field menu", rows[0][0])
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

// linkTapID is the ID of the /start message a link tap sends in the tests.
const linkTapID = 900

func TestLinkTapDeletesStart(t *testing.T) {
	tests := []struct {
		name, text string
		review     bool
		failDelete bool
		wantNotice bool
	}{
		{name: "edit link", text: "/start edit_1234567890_42"},
		{name: "edit link to no giveaway", text: "/start edit_1234567890_43", wantNotice: true},
		{name: "results link to no giveaway", text: "/start results_1234567890_43", review: true, wantNotice: true},
		{name: "delete fails", text: "/start edit_1234567890_42", failDelete: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandlerSetup(t)
			h.api.failDelete = tt.failDelete
			publish(
				t,
				h.store,
				&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"},
			)

			u := h.message(tt.text, false)
			u.Message.ID = linkTapID

			log := slog.New(slog.DiscardHandler)
			handler := giveaway.EditLinkHandler(h.ed, h.texts, h.users, h.kb, log)
			if tt.review {
				store := resultsStore{memStore: h.store}
				rv := giveaway.NewReviewer(
					store,
					store,
					store,
					noChannels{},
					giveaway.AcceptAll,
					rand.Shuffle,
					h.texts,
					time.Now,
					log,
				)
				handler = giveaway.ReviewLinkHandler(rv, h.texts, h.users, h.kb, log)
			}

			handler(t.Context(), h.bot, u)

			if !slices.Contains(h.deleted(), strconv.Itoa(linkTapID)) {
				t.Errorf("deleted %v, want the /start message %d", h.deleted(), linkTapID)
			}

			if sent := h.sent(); len(sent) != 1 {
				t.Fatalf("sent %q, want one reply", sent)
			}

			if !tt.wantNotice && len(h.keyboard(t)) == 0 {
				t.Error("no menu buttons, want the field menu")
			}
		})
	}
}

func TestFieldLinkHandler(t *testing.T) {
	tests := []struct {
		name     string
		payload  string // follows "/start field_"; <id> stands for the giveaway ID
		from     int64
		wantStep string // the step the link starts; "" for none
		notice   i18n.Key
	}{
		{name: "winners", payload: "winners_<id>", wantStep: "edit:winners:<id>"},
		{name: "field with underscore", payload: "end_date_<id>", wantStep: "edit:end_date:<id>"},
		{name: "button text", payload: "button_text_<id>", wantStep: "edit:button_text:<id>"},
		{name: "post", payload: "post_<id>", wantStep: "edit:move:<id>"},
		{name: "another user", payload: "winners_<id>", from: adminID + 1, notice: i18n.EditNotYours},
		{name: "unknown giveaway", payload: "winners_999", notice: i18n.EditNotFound},
		{name: "unknown field", payload: "title_<id>", notice: i18n.EditNotFound},
		{name: "zero ID", payload: "winners_0", notice: i18n.EditNotFound},
		{name: "no ID", payload: "winners", notice: i18n.EditNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandlerSetup(t)
			g := publish(
				t,
				h.store,
				&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"},
			)
			id := strconv.FormatUint(uint64(g.ID), 10)

			u := h.message(giveaway.FieldLinkPrefix+strings.ReplaceAll(tt.payload, "<id>", id), false)
			u.Message.ID = linkTapID
			if tt.from != 0 {
				u.Message.From.ID = tt.from
				u.Message.Chat.ID = tt.from
			}

			giveaway.FieldLinkHandler(h.ed, h.texts, h.users, h.kb, slog.New(slog.DiscardHandler))(
				t.Context(), h.bot, u,
			)

			if !slices.Contains(h.deleted(), strconv.Itoa(linkTapID)) {
				t.Errorf("deleted %v, want the /start message %d", h.deleted(), linkTapID)
			}

			if tt.notice != "" {
				want := h.texts.Text("en", tt.notice)
				if got := h.sent(); len(got) != 1 || got[0] != want {
					t.Errorf("sent %q, want only %q", got, want)
				}

				return
			}

			if got, want := h.users.pending[adminID], strings.ReplaceAll(tt.wantStep, "<id>", id); got != want {
				t.Errorf("pending step = %q, want %q", got, want)
			}

			rows := h.keyboard(t)
			if last := rows[len(rows)-1][0]; last.CallbackData != "giveaway-edit:stop" {
				t.Errorf("last button = %+v, want Cancel", last)
			}
		})
	}
}

func TestStartLinkPrefixes(t *testing.T) {
	prefixes := []string{giveaway.EditLinkPrefix, giveaway.ReviewLinkPrefix, giveaway.FieldLinkPrefix}
	for _, a := range prefixes {
		for _, b := range prefixes {
			if a != b && strings.HasPrefix(a, b) {
				t.Errorf("%q starts with %q, so its links reach the wrong handler", a, b)
			}
		}
	}

	// A start payload holds at most 64 characters from A-Z, a-z, 0-9, _ and -.
	valid := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	longest := strings.TrimPrefix(giveaway.FieldLinkPrefix, "/start ") + giveaway.FieldButtonText + "_4294967295"
	if !valid.MatchString(longest) {
		t.Errorf("field link payload %q is not a valid start payload", longest)
	}
}

func TestEditOldFieldButtonIsStale(t *testing.T) {
	h := newHandlerSetup(t)
	g := h.openMenu(t)
	pending := h.users.pending[adminID]

	h.tap(t, "giveaway-edit:field:winners:"+strconv.FormatUint(uint64(g.ID), 10))

	if got := h.users.pending[adminID]; got != pending {
		t.Errorf("pending step = %q, want %q kept: a field button from an older menu", got, pending)
	}
}

func TestEditMenuSplitsEndDateAndTime(t *testing.T) {
	h := newHandlerSetup(t)
	g := publish(t, h.store, &giveaway.Giveaway{
		ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: h.kyivTime(25, 19, 0),
	})
	id := strconv.FormatUint(uint64(g.ID), 10)

	text, _, err := h.ed.Menu(t.Context(), g.ID, "en", "test_bot")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"<b>📅 Date:</b> 2026-10-25 " + `<a href="https://t.me/test_bot?start=field_end_date_` + id + `">`,
		"<b>⏰ Time:</b> 19:00 (Kyiv time) " + `<a href="https://t.me/test_bot?start=field_end_time_` + id + `">`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("menu = %q, want %q", text, want)
		}
	}
}
