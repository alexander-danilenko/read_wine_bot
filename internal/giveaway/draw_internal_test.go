package giveaway

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

const (
	drawChannel int64 = -1_001_234_567_890
	drawPost    int64 = 42
	creatorID   int64 = 7_000_000_001
)

var errDraw = errors.New("boom")

// fakeDrawStore serves due giveaways and their candidates and records what Finish saved.
type fakeDrawStore struct {
	due        []Giveaway
	dueErr     error
	dueCalls   int
	candidates map[uint][]user.User
	finishErr  error
	finished   map[uint][]int64
}

func (f *fakeDrawStore) Due(context.Context, time.Time) ([]Giveaway, error) {
	f.dueCalls++

	return f.due, f.dueErr
}

func (f *fakeDrawStore) Candidates(_ context.Context, giveawayID uint) ([]user.User, error) {
	return f.candidates[giveawayID], nil
}

func (f *fakeDrawStore) Finish(_ context.Context, id uint, winnerIDs []int64, _ time.Time) error {
	if f.finishErr != nil {
		return f.finishErr
	}

	if f.finished == nil {
		f.finished = map[uint][]int64{}
	}

	f.finished[id] = winnerIDs

	return nil
}

// sent is one message the fake announcer was asked to send.
type sent struct {
	chatID, replyTo int64
	text            string
	ctxErr          error // the context's error when the message was sent
}

// fakeAnnouncer records the calls the draw makes to Telegram and fails the chats in failChats.
type fakeAnnouncer struct {
	removeErr error
	removed   int
	failChats map[int64]bool
	sent      []sent
}

func (f *fakeAnnouncer) RemoveButton(context.Context, int64, int64) error {
	f.removed++

	return f.removeErr
}

func (f *fakeAnnouncer) SendHTML(ctx context.Context, chatID, replyTo int64, text string) error {
	if f.failChats[chatID] {
		return errDraw
	}

	f.sent = append(f.sent, sent{chatID: chatID, replyTo: replyTo, text: text, ctxErr: ctx.Err()})

	return nil
}

func dueGiveaway(id uint, winners int) Giveaway {
	return Giveaway{
		ID: id, ChatID: drawChannel, MessageID: drawPost, CreatedBy: creatorID,
		Language: "en", Winners: winners, Published: true,
	}
}

func users(ids ...int64) []user.User {
	us := make([]user.User, 0, len(ids))
	for _, id := range ids {
		us = append(us, user.User{TelegramID: id, FirstName: "User"})
	}

	return us
}

func noShuffle(int, func(i, j int)) {}

func reverse(n int, swap func(i, j int)) {
	for i := range n / 2 {
		swap(i, n-1-i)
	}
}

func newDrawer(
	t *testing.T,
	store DrawStore,
	filter Filter,
	shuffle func(int, func(i, j int)),
) (*Drawer, *bytes.Buffer) {
	t.Helper()

	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer

	now := func() time.Time { return time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC) }
	log := slog.New(slog.NewTextHandler(&logs, nil))

	return NewDrawer(store, filter, shuffle, texts, now, log), &logs
}

func TestDrawPicksWinners(t *testing.T) {
	rejectSecond := func(_ context.Context, _ Giveaway, u user.User) (bool, error) { return u.TelegramID != 2, nil }
	rejectAll := func(context.Context, Giveaway, user.User) (bool, error) { return false, nil }

	tests := []struct {
		name       string
		winners    int
		candidates []user.User
		filter     Filter
		shuffle    func(int, func(i, j int))
		want       []int64
	}{
		{"first in shuffled order", 2, users(1, 2, 3, 4, 5), AcceptAll, noShuffle, []int64{1, 2}},
		{"fewer candidates than winners", 3, users(1, 2), AcceptAll, noShuffle, []int64{1, 2}},
		{"nobody joined", 1, nil, AcceptAll, noShuffle, []int64{}},
		{"distinct users", 3, users(1, 2, 3), AcceptAll, reverse, []int64{3, 2, 1}},
		{"rejected candidate replaced", 2, users(1, 2, 3), rejectSecond, noShuffle, []int64{1, 3}},
		{"everyone rejected", 2, users(1, 2), rejectAll, noShuffle, []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeDrawStore{
				due:        []Giveaway{dueGiveaway(1, tt.winners)},
				candidates: map[uint][]user.User{1: tt.candidates},
			}
			d, _ := newDrawer(t, store, tt.filter, tt.shuffle)

			d.drawDue(t.Context(), &fakeAnnouncer{})

			got, ok := store.finished[1]
			if !ok || !slices.Equal(got, tt.want) {
				t.Errorf("finished with winners %v (saved: %v), want %v", got, ok, tt.want)
			}
		})
	}
}

func TestDrawFilterErrorLeavesGiveawayDue(t *testing.T) {
	failOnFirst := func(_ context.Context, g Giveaway, _ user.User) (bool, error) {
		if g.ID == 1 {
			return false, errDraw
		}

		return true, nil
	}
	store := &fakeDrawStore{
		due:        []Giveaway{dueGiveaway(1, 1), dueGiveaway(2, 1)},
		candidates: map[uint][]user.User{1: users(1), 2: users(2)},
	}
	d, logs := newDrawer(t, store, failOnFirst, noShuffle)

	d.drawDue(t.Context(), &fakeAnnouncer{})

	if _, ok := store.finished[1]; ok {
		t.Error("giveaway 1 was finished although its filter failed")
	}

	if got := store.finished[2]; !slices.Equal(got, []int64{2}) {
		t.Errorf("giveaway 2 winners = %v, want [2]", got)
	}

	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("filter error not logged: %s", logs)
	}
}

func TestDrawSkipsGiveawayNoLongerDue(t *testing.T) {
	store := &fakeDrawStore{due: []Giveaway{dueGiveaway(1, 1)}, finishErr: ErrNotFound}
	tg := &fakeAnnouncer{}
	d, logs := newDrawer(t, store, AcceptAll, noShuffle)

	d.drawDue(t.Context(), tg)

	if tg.removed != 0 || len(tg.sent) != 0 {
		t.Errorf("announced a giveaway that was not finished: removed %d, sent %d", tg.removed, len(tg.sent))
	}

	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("ErrNotFound logged as an error: %s", logs)
	}
}

func TestDrawDueErrorIsLogged(t *testing.T) {
	d, logs := newDrawer(t, &fakeDrawStore{dueErr: errDraw}, AcceptAll, noShuffle)

	d.drawDue(t.Context(), &fakeAnnouncer{})

	if got := strings.Count(logs.String(), "level=ERROR"); got != 1 {
		t.Errorf("logged %d errors, want 1: %s", got, logs)
	}
}

func TestDrawAnnouncesInChannel(t *testing.T) {
	store := &fakeDrawStore{
		due:        []Giveaway{dueGiveaway(1, 1)},
		candidates: map[uint][]user.User{1: users(5)},
	}
	tg := &fakeAnnouncer{}
	d, _ := newDrawer(t, store, AcceptAll, noShuffle)

	d.drawDue(t.Context(), tg)

	if tg.removed != 1 {
		t.Errorf("join button removed %d times, want 1", tg.removed)
	}

	if len(tg.sent) != 1 || tg.sent[0].chatID != drawChannel || tg.sent[0].replyTo != drawPost ||
		!strings.Contains(tg.sent[0].text, `href="tg://user?id=5"`) {
		t.Errorf("sent %+v, want one reply to the post linking user 5", tg.sent)
	}
}

func TestDrawAnnouncesDespiteButtonError(t *testing.T) {
	store := &fakeDrawStore{due: []Giveaway{dueGiveaway(1, 1)}, candidates: map[uint][]user.User{1: users(5)}}
	tg := &fakeAnnouncer{removeErr: errDraw}
	d, logs := newDrawer(t, store, AcceptAll, noShuffle)

	d.drawDue(t.Context(), tg)

	if len(tg.sent) != 1 {
		t.Errorf("sent %d messages, want the results anyway", len(tg.sent))
	}

	if !strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("button error not logged as a warning: %s", logs)
	}
}

func TestDrawFallsBackToCreator(t *testing.T) {
	store := &fakeDrawStore{
		due:        []Giveaway{dueGiveaway(1, 1)},
		candidates: map[uint][]user.User{1: users(5)},
	}
	tg := &fakeAnnouncer{failChats: map[int64]bool{drawChannel: true}}
	d, _ := newDrawer(t, store, AcceptAll, noShuffle)

	d.drawDue(t.Context(), tg)

	if len(tg.sent) != 2 {
		t.Fatalf("sent %+v, want the post link and the results to the creator", tg.sent)
	}

	link, results := tg.sent[0], tg.sent[1]
	if link.chatID != creatorID || link.replyTo != 0 || !strings.Contains(link.text, postLink(drawChannel, drawPost)) {
		t.Errorf("first private message = %+v, want the post link to the creator", link)
	}

	if results.chatID != creatorID || results.replyTo != 0 || !strings.Contains(results.text, `href="tg://user?id=5"`) {
		t.Errorf("second private message = %+v, want the results to the creator", results)
	}

	if _, ok := store.finished[1]; !ok {
		t.Error("giveaway not finished")
	}
}

func TestDrawLogsWhenEveryAnnouncementFails(t *testing.T) {
	store := &fakeDrawStore{
		due:        []Giveaway{dueGiveaway(1, 1), dueGiveaway(2, 1)},
		candidates: map[uint][]user.User{1: users(5), 2: users(6)},
	}
	tg := &fakeAnnouncer{failChats: map[int64]bool{drawChannel: true, creatorID: true}}
	d, logs := newDrawer(t, store, AcceptAll, noShuffle)

	d.drawDue(t.Context(), tg)

	if got := strings.Count(logs.String(), "level=ERROR"); got != 2 {
		t.Errorf("logged %d errors, want one per giveaway: %s", got, logs)
	}

	if len(store.finished) != 2 {
		t.Errorf("finished %d giveaways, want the pass to go on to the second", len(store.finished))
	}
}

func TestDrawAnnouncesAfterCancel(t *testing.T) {
	store := &fakeDrawStore{due: []Giveaway{dueGiveaway(1, 1)}, candidates: map[uint][]user.User{1: users(5)}}
	ctx, cancel := context.WithCancel(t.Context())
	// Shutdown arrives while the winners are being picked, so after Due and before the
	// announcement.
	filter := func(context.Context, Giveaway, user.User) (bool, error) {
		cancel()

		return true, nil
	}
	tg := &fakeAnnouncer{}
	d, _ := newDrawer(t, store, filter, noShuffle)

	d.drawDue(ctx, tg)

	if len(tg.sent) != 1 || tg.sent[0].ctxErr != nil {
		t.Errorf("sent %+v, want the results sent with a live context", tg.sent)
	}
}

func visibleLen(s string) int {
	// The tests strip the only markup the builder emits: the winner links.
	for {
		start := strings.Index(s, "<a ")
		if start < 0 {
			break
		}

		end := strings.Index(s[start:], ">")
		s = s[:start] + s[start+end+1:]
	}

	s = strings.ReplaceAll(s, "</a>", "")
	for _, e := range []string{"&lt;", "&gt;", "&amp;", "&#34;", "&#39;"} {
		s = strings.ReplaceAll(s, e, "x")
	}

	return len(utf16.Encode([]rune(s)))
}

func TestResultMessages(t *testing.T) {
	d, _ := newDrawer(t, &fakeDrawStore{}, AcceptAll, noShuffle)

	got := d.resultMessages("en", []user.User{
		{TelegramID: 1, FirstName: "Ann", LastName: "Smith"},
		{TelegramID: 2, FirstName: "Bob"},
		{TelegramID: 3, FirstName: "<b>&"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}

	for _, want := range []string{
		`<a href="tg://user?id=1">Ann Smith</a>`,
		`<a href="tg://user?id=2">Bob</a>`,
		`<a href="tg://user?id=3">&lt;b&gt;&amp;</a>`,
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("message %q does not contain %q", got[0], want)
		}
	}

	if !strings.HasPrefix(got[0], d.texts.Text("en", i18n.DrawResults)) {
		t.Errorf("message %q does not start with the header", got[0])
	}
}

func TestResultMessagesWithoutWinners(t *testing.T) {
	d, _ := newDrawer(t, &fakeDrawStore{}, AcceptAll, noShuffle)

	got := d.resultMessages("uk", nil)
	if len(got) != 1 || got[0] != d.texts.Text("uk", i18n.DrawNoWinners) {
		t.Errorf("got %q, want only the no-winners text", got)
	}
}

func TestResultMessagesSplitLongResults(t *testing.T) {
	d, _ := newDrawer(t, &fakeDrawStore{}, AcceptAll, noShuffle)

	long := strings.Repeat("😀", 64) // 2 UTF-16 code units each
	winners := make([]user.User, 0, maxWinners)
	for i := range int64(maxWinners) {
		winners = append(winners, user.User{TelegramID: i + 1, FirstName: long, LastName: long})
	}

	got := d.resultMessages("en", winners)
	if len(got) < 2 {
		t.Fatalf("got %d messages, want the results split", len(got))
	}

	header := d.texts.Text("en", i18n.DrawResults)
	links := 0

	for i, m := range got {
		if n := visibleLen(m); n > maxMessageLen {
			t.Errorf("message %d is %d UTF-16 code units long, over %d", i, n, maxMessageLen)
		}

		if strings.HasPrefix(m, header) != (i == 0) {
			t.Errorf("message %d: header present = %v, want only in the first", i, i != 0)
		}

		links += strings.Count(m, "tg://user?id=")
	}

	if links != maxWinners {
		t.Errorf("messages link %d winners, want %d", links, maxWinners)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	store := &fakeDrawStore{}
	d, _ := newDrawer(t, store, AcceptAll, noShuffle)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d.run(ctx, &fakeAnnouncer{}, time.Hour)

	if store.dueCalls != 0 {
		t.Errorf("drew %d times on a cancelled context, want 0", store.dueCalls)
	}
}

func TestRunDrawsAtOnceThenEveryTick(t *testing.T) {
	store := &fakeDrawStore{}
	d, _ := newDrawer(t, store, AcceptAll, noShuffle)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	d.run(ctx, &fakeAnnouncer{}, 20*time.Millisecond)

	if store.dueCalls < 2 {
		t.Errorf("drew %d times in 50ms with a 20ms tick, want a pass at once and at least one tick", store.dueCalls)
	}
}
