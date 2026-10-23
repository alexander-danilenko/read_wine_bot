package giveaway

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

const reviewID uint = 1

// participant is one row of the fake giveaway_participants table.
type participant struct {
	user     user.User
	winner   bool
	place    int // 0 for NULL
	rerolled bool
}

// fakeReviewStore keeps one channel's giveaways and their participants in memory and follows
// the contract documented on *Store, including the constraints the database enforces:
// chk_participants_place, chk_participants_rerolled and idx_participants_place.
type fakeReviewStore struct {
	mu           sync.Mutex
	giveaways    map[uint]*Giveaway
	participants map[uint][]*participant // in join order
	onClaim      func()                  // runs inside ClaimResults when set
}

func newFakeReviewStore() *fakeReviewStore {
	return &fakeReviewStore{giveaways: map[uint]*Giveaway{}, participants: map[uint][]*participant{}}
}

func (f *fakeReviewStore) ByID(_ context.Context, id uint) (Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if g, ok := f.giveaways[id]; ok {
		return *g, nil
	}

	return Giveaway{}, ErrNotFound
}

func (f *fakeReviewStore) Post(_ context.Context, chatID, messageID int64) (Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, g := range f.giveaways {
		if g.ChatID == chatID && g.MessageID == messageID {
			return *g, nil
		}
	}

	return Giveaway{}, ErrNotFound
}

func (f *fakeReviewStore) Results(_ context.Context, id uint) (Results, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var r Results

	for _, p := range f.participants[id] {
		if p.winner {
			r.Winners = append(r.Winners, Winner{User: p.user, Place: p.place})
		}

		if p.rerolled {
			r.Rerolled++
		}
	}

	slices.SortStableFunc(r.Winners, func(a, b Winner) int { return a.Place - b.Place })

	return r, nil
}

func (f *fakeReviewStore) Eligible(_ context.Context, id uint) ([]user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var us []user.User

	for _, p := range f.participants[id] {
		if !p.winner && !p.rerolled {
			us = append(us, p.user)
		}
	}

	return us, nil
}

func (f *fakeReviewStore) Reroll(_ context.Context, id uint, out, in int64, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	g, ok := f.giveaways[id]
	if !ok || !g.ResultsPending {
		return ErrNotFound
	}

	o := f.find(id, out)
	if o == nil || !o.winner {
		return ErrNotFound
	}

	var n *participant
	if in != 0 {
		if n = f.find(id, in); n == nil || n.winner || n.rerolled {
			return ErrNotFound // the transaction rolls back: nothing changed
		}
	}

	place := o.place
	o.winner, o.place, o.rerolled = false, 0, true

	if n != nil {
		n.winner, n.place = true, place
	}

	f.check(id)

	return nil
}

func (f *fakeReviewStore) ClaimResults(_ context.Context, id uint, rerolled int) error {
	if f.onClaim != nil {
		f.onClaim()
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	g, ok := f.giveaways[id]
	if !ok || !g.ResultsPending || f.rerolled(id) != rerolled {
		return ErrNotFound
	}

	g.ResultsPending = false

	return nil
}

func (f *fakeReviewStore) ReleaseResults(_ context.Context, id uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if g, ok := f.giveaways[id]; ok && g.FinishedAt != nil {
		g.ResultsPending = true
	}

	return nil
}

// drawn adds a giveaway whose results wait for review, with the given winners at places 1,
// 2, … and the other participants.
func (f *fakeReviewStore) drawn(winners []user.User, others ...user.User) {
	now := time.Now()
	f.giveaways[reviewID] = &Giveaway{
		ID: reviewID, ChatID: drawChannel, MessageID: drawPost, CreatedBy: creatorID,
		Language: "en", Winners: max(1, len(winners)), Published: true, FinishedAt: &now, ResultsPending: true,
	}

	for i, u := range winners {
		f.participants[reviewID] = append(f.participants[reviewID], &participant{user: u, winner: true, place: i + 1})
	}

	for _, u := range others {
		f.participants[reviewID] = append(f.participants[reviewID], &participant{user: u})
	}
}

// check panics when a row breaks a constraint the database enforces, so a test fails loudly.
func (f *fakeReviewStore) check(id uint) {
	places := map[int]bool{}

	for _, p := range f.participants[id] {
		if p.place != 0 && !p.winner {
			panic("chk_participants_place: a place on a participant who is not a winner")
		}

		if p.rerolled && p.winner {
			panic("chk_participants_rerolled: a rerolled-out winner")
		}

		if p.place != 0 && places[p.place] {
			panic("idx_participants_place: two winners at one place")
		}

		places[p.place] = true
	}

	if g := f.giveaways[id]; g.ResultsPending && g.FinishedAt == nil {
		panic("chk_giveaways_results: pending results before the draw")
	}
}

func (f *fakeReviewStore) find(id uint, userID int64) *participant {
	for _, p := range f.participants[id] {
		if p.user.TelegramID == userID {
			return p
		}
	}

	return nil
}

func (f *fakeReviewStore) rerolled(id uint) int {
	n := 0

	for _, p := range f.participants[id] {
		if p.rerolled {
			n++
		}
	}

	return n
}

func (f *fakeReviewStore) pending(t *testing.T) bool {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.giveaways[reviewID].ResultsPending
}

// posted is one message the fake Telegram was asked to send.
type posted struct {
	chatID, replyTo int64
	text            string
	ctxErr          error
}

// fakeReviewTelegram answers admin checks and records sent messages. Admins are the users in
// admins; the sendFails-th message (from 1) fails.
type fakeReviewTelegram struct {
	admins    map[int64]bool
	adminErr  error
	sendFails int
	sent      []posted
	attempts  int
}

func (f *fakeReviewTelegram) IsChannelAdmin(_ context.Context, _, userID int64) (bool, error) {
	return f.admins[userID], f.adminErr
}

func (f *fakeReviewTelegram) SendHTML(ctx context.Context, chatID, replyTo int64, text string) error {
	f.attempts++
	if f.attempts == f.sendFails {
		return errDraw
	}

	f.sent = append(f.sent, posted{chatID: chatID, replyTo: replyTo, text: text, ctxErr: ctx.Err()})

	return nil
}

func newAdmins() *fakeReviewTelegram {
	return &fakeReviewTelegram{admins: map[int64]bool{creatorID: true, otherAdmin: true}}
}

const otherAdmin int64 = 7_000_000_009

func named(id int64, name string) user.User { return user.User{TelegramID: id, FirstName: name} }

func ann() user.User { return named(1, "Ann") }

func bob() user.User { return named(2, "Bob") }

func cid() user.User { return named(3, "Cid") }

func dan() user.User { return named(4, "Dan") }

func newTestReviewer(t *testing.T, store *fakeReviewStore) *Reviewer {
	t.Helper()

	rv, _ := newLoggedReviewer(t, store, AcceptAll, noShuffle)

	return rv
}

func newLoggedReviewer(
	t *testing.T, store *fakeReviewStore, filter Filter, shuffle func(int, func(i, j int)),
) (*Reviewer, *bytes.Buffer) {
	t.Helper()

	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer

	now := func() time.Time { return time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC) }

	return NewReviewer(
		store,
		store,
		store,
		filter,
		shuffle,
		texts,
		now,
		slog.New(slog.NewTextHandler(&logs, nil)),
	), &logs
}

func winnerIDs(t *testing.T, store *fakeReviewStore) map[int]int64 {
	t.Helper()

	res, err := store.Results(t.Context(), reviewID)
	if err != nil {
		t.Fatal(err)
	}

	got := map[int]int64{}
	for _, w := range res.Winners {
		got[w.Place] = w.TelegramID
	}

	return got
}

// Covers AE1.
func TestRerollKeepsPlaceAndNeverPicksRerolledOut(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann(), bob()}, cid(), dan())
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	r := rv.Reroll(t.Context(), tg, creatorID, reviewID, ann().TelegramID)
	if r.Stale || r.Notice != "" || r.Screen != ResultsScreen {
		t.Fatalf("first reroll = %+v, want the review again", r)
	}

	if got := winnerIDs(t, store); got[1] != cid().TelegramID || got[2] != bob().TelegramID {
		t.Fatalf("winners after first reroll = %v, want Cid at 1 and Bob at 2", got)
	}

	rv.Reroll(t.Context(), tg, creatorID, reviewID, cid().TelegramID)

	if got := winnerIDs(t, store); got[1] != dan().TelegramID {
		t.Errorf("winner 1 after second reroll = %d, want Dan, the only eligible participant", got[1])
	}
}

// Covers AE2.
func TestRerollWithoutReplacementLeavesFewerWinners(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()})
	rv := newTestReviewer(t, store)

	r := rv.Reroll(t.Context(), newAdmins(), creatorID, reviewID, ann().TelegramID)
	if len(r.Results.Winners) != 0 {
		t.Fatalf("winners = %+v, want none", r.Results.Winners)
	}

	text, rows := reviewView(rv, "en", r)
	if text != rv.texts.Text("en", i18n.ReviewNoWinners, postLink(drawChannel, drawPost)) {
		t.Errorf("review text = %q, want the no-winners text", text)
	}

	if len(rows) != 1 || len(rows[0]) != 1 ||
		!strings.HasPrefix(rows[0][0].CallbackData, ReviewCallbackPrefix+reviewAskPublish) {
		t.Errorf("rows = %+v, want only the publish button", rows)
	}
}

// Covers AE3.
func TestSecondRerollOfSameWinnerIsStale(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()}, cid(), dan())
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	rv.Reroll(t.Context(), tg, creatorID, reviewID, ann().TelegramID)
	before := winnerIDs(t, store)

	if r := rv.Reroll(t.Context(), tg, creatorID, reviewID, ann().TelegramID); !r.Stale {
		t.Errorf("second reroll = %+v, want stale", r)
	}

	if got := winnerIDs(t, store); got[1] != before[1] {
		t.Errorf("winner changed to %d on a stale tap, want %d", got[1], before[1])
	}
}

// Covers AE4.
func TestRerollAfterPublishIsStale(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()}, cid())
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	if r := rv.Publish(t.Context(), tg, otherAdmin, reviewID, 0); r.Notice != i18n.ReviewPublished {
		t.Fatalf("publish = %+v, want published", r)
	}

	if r := rv.Reroll(t.Context(), tg, creatorID, reviewID, ann().TelegramID); !r.Stale {
		t.Errorf("reroll after publish = %+v, want stale", r)
	}

	if r := rv.Publish(t.Context(), tg, creatorID, reviewID, 0); !r.Stale {
		t.Errorf("second publish = %+v, want stale", r)
	}

	if len(tg.sent) != 1 {
		t.Errorf("sent %d messages, want the results once", len(tg.sent))
	}
}

func TestPublishPostsResultsUnderThePost(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann(), bob()})
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	r := rv.Publish(t.Context(), tg, creatorID, reviewID, 0)
	if r.Notice != i18n.ReviewPublished || r.Screen != NoScreen {
		t.Errorf("publish = %+v, want the published notice", r)
	}

	if len(tg.sent) != 1 || tg.sent[0].chatID != drawChannel || tg.sent[0].replyTo != drawPost ||
		!strings.Contains(
			tg.sent[0].text,
			`href="tg://user?id=1"`,
		) || !strings.Contains(tg.sent[0].text, `href="tg://user?id=2"`) {
		t.Errorf("sent %+v, want one reply to the post linking Ann and Bob", tg.sent)
	}

	if store.pending(t) {
		t.Error("results still pending after publishing")
	}
}

// Covers AE5.
func TestPublishRefusedByChannelKeepsResultsPending(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()})
	rv := newTestReviewer(t, store)
	tg := newAdmins()
	tg.sendFails = 1

	r := rv.Publish(t.Context(), tg, creatorID, reviewID, 0)
	if r.Notice != i18n.ReviewPublishFailed || r.Screen != ResultsScreen {
		t.Errorf("publish = %+v, want the refusal notice and the review again", r)
	}

	if !store.pending(t) {
		t.Error("results not pending after the channel refused them")
	}
}

func TestPublishPartlyPosted(t *testing.T) {
	store := newFakeReviewStore()
	long := strings.Repeat("😀", 64)
	winners := make([]user.User, 0, maxWinners)

	for i := range int64(maxWinners) {
		winners = append(winners, user.User{TelegramID: i + 1, FirstName: long, LastName: long})
	}

	store.drawn(winners)
	rv := newTestReviewer(t, store)
	tg := newAdmins()
	tg.sendFails = 2

	if r := rv.Publish(t.Context(), tg, creatorID, reviewID, 0); r.Notice != i18n.ReviewPublishedPartly {
		t.Errorf("publish = %+v, want the partly-posted notice", r)
	}

	if store.pending(t) {
		t.Error("results pending again although part of them is in the channel")
	}
}

func TestPublishAfterAnotherAdminsRerollIsStale(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann(), bob()}, cid())
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	ask := rv.AskPublish(t.Context(), tg, creatorID, reviewID, 0)
	if ask.Screen != PublishScreen {
		t.Fatalf("ask publish = %+v, want the confirmation", ask)
	}

	rv.Reroll(t.Context(), tg, otherAdmin, reviewID, bob().TelegramID)

	if r := rv.Publish(t.Context(), tg, creatorID, reviewID, ask.Results.Rerolled); !r.Stale {
		t.Errorf("publish of the older review = %+v, want stale", r)
	}

	if len(tg.sent) != 0 || !store.pending(t) {
		t.Errorf(
			"sent %d messages, pending %v; want nothing posted and the results pending",
			len(tg.sent),
			store.pending(t),
		)
	}
}

func TestPublishPostsDespiteShutdownAfterClaim(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()})
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	ctx, cancel := context.WithCancel(t.Context())
	store.onClaim = cancel

	rv.Publish(ctx, tg, creatorID, reviewID, 0)

	if len(tg.sent) != 1 || tg.sent[0].ctxErr != nil {
		t.Errorf("sent %+v, want the results posted with a live context", tg.sent)
	}
}

func TestReviewAccess(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
		tg     *fakeReviewTelegram
		want   i18n.Key
	}{
		{"not an admin", fanUser, newAdmins(), i18n.ReviewNotAdmin},
		{"admin check fails", creatorID, &fakeReviewTelegram{adminErr: errDraw}, i18n.EditFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeReviewStore()
			store.drawn([]user.User{ann()}, cid())
			rv := newTestReviewer(t, store)

			for _, r := range []ReviewReply{
				rv.Show(t.Context(), tt.tg, tt.userID, reviewID),
				rv.Reroll(t.Context(), tt.tg, tt.userID, reviewID, ann().TelegramID),
				rv.Publish(t.Context(), tt.tg, tt.userID, reviewID, 0),
			} {
				if r.Notice != tt.want || r.Screen != NoScreen {
					t.Errorf("reply = %+v, want only notice %q", r, tt.want)
				}
			}

			if got := winnerIDs(t, store); got[1] != ann().TelegramID || !store.pending(t) || len(tt.tg.sent) != 0 {
				t.Errorf("state changed: winners %v, pending %v, sent %d", got, store.pending(t), len(tt.tg.sent))
			}
		})
	}
}

const fanUser int64 = 7_000_000_002

func TestRerollFilterErrorChangesNothing(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()}, cid())
	failing := func(context.Context, Giveaway, user.User) (bool, error) { return false, errDraw }
	rv, logs := newLoggedReviewer(t, store, failing, noShuffle)

	r := rv.Reroll(t.Context(), newAdmins(), creatorID, reviewID, ann().TelegramID)
	if r.Notice != i18n.EditFailed {
		t.Errorf("reroll = %+v, want the failure notice", r)
	}

	if got := winnerIDs(t, store); got[1] != ann().TelegramID {
		t.Errorf("winner 1 = %d after a failed filter, want Ann", got[1])
	}

	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("filter error not logged: %s", logs)
	}
}

func TestOpenFromLink(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()})
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	if r := rv.Open(t.Context(), tg, otherAdmin, drawChannel, drawPost); r.Screen != ResultsScreen {
		t.Errorf("open = %+v, want the review", r)
	}

	if r := rv.Open(t.Context(), tg, creatorID, drawChannel, drawPost+1); r.Notice != i18n.EditNotFound {
		t.Errorf("open of another post = %+v, want not found", r)
	}

	rv.Publish(t.Context(), tg, creatorID, reviewID, 0)

	if r := rv.Open(t.Context(), tg, creatorID, drawChannel, drawPost); r.Notice != i18n.ReviewNotPending {
		t.Errorf("open after publishing = %+v, want already published", r)
	}
}

func TestReviewFitsOneMessageWithMostWinners(t *testing.T) {
	store := newFakeReviewStore()
	long := strings.Repeat("Ж", 64)
	winners := make([]user.User, 0, maxWinners)

	for i := range int64(maxWinners) {
		winners = append(winners, user.User{TelegramID: i + 1, FirstName: long, LastName: long})
	}

	store.drawn(winners)
	rv := newTestReviewer(t, store)

	r := rv.Show(t.Context(), newAdmins(), creatorID, reviewID)
	text, rows := reviewView(rv, "en", r)

	if n := visibleLen(text); n > maxMessageLen {
		t.Errorf("review is %d UTF-16 code units long, over %d", n, maxMessageLen)
	}

	if n := strings.Count(text, "tg://user?id="); n != maxWinners {
		t.Errorf("review links %d winners, want %d", n, maxWinners)
	}

	buttons := 0
	for _, row := range rows {
		buttons += len(row)
	}

	if buttons > 100 {
		t.Errorf("review has %d buttons, over Telegram's 100", buttons)
	}
}

func TestReviewNamesWinnerToReroll(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann(), bob()}, cid())
	rv := newTestReviewer(t, store)

	r := rv.AskReroll(t.Context(), newAdmins(), creatorID, reviewID, bob().TelegramID)
	text, rows := reviewView(rv, "en", r)

	if !strings.Contains(text, "2") || !strings.Contains(text, `href="tg://user?id=2">Bob</a>`) {
		t.Errorf("confirmation = %q, want winner 2, Bob", text)
	}

	if len(rows) != 1 || rows[0][0].CallbackData != reviewData(reviewReroll, reviewID, bob().TelegramID) ||
		rows[0][1].CallbackData != reviewData(reviewShow, reviewID) {
		t.Errorf("rows = %+v, want Yes for Bob and back to the review", rows)
	}

	if r := rv.AskReroll(t.Context(), newAdmins(), creatorID, reviewID, cid().TelegramID); !r.Stale {
		t.Errorf("ask to reroll a participant who is not a winner = %+v, want stale", r)
	}
}

func TestReviewCallbackData(t *testing.T) {
	const maxID = ^uint(0)

	const maxUser int64 = 1<<63 - 1

	for _, data := range []string{
		reviewData(reviewShow, maxID),
		reviewData(reviewAskReroll, maxID, maxUser),
		reviewData(reviewReroll, maxID, maxUser),
		reviewData(reviewAskPublish, maxID, maxWinners),
		reviewData(reviewPublish, maxID, maxUser),
	} {
		if len(data) > 64 {
			t.Errorf("%q is %d bytes, over Telegram's 64", data, len(data))
		}

		for _, other := range []string{EditCallbackPrefix, SetupCallbackPrefix, JoinCallback} {
			if strings.HasPrefix(data, other) || strings.HasPrefix(other, ReviewCallbackPrefix) {
				t.Errorf("%q overlaps prefix %q", data, other)
			}
		}

		a, ok := parseReviewData(data)
		if !ok || a.id != maxID {
			t.Errorf("parse %q = %+v, %v; want it back", data, a, ok)
		}
	}

	for _, data := range []string{
		ReviewCallbackPrefix + "pub:1",
		ReviewCallbackPrefix + "show:x",
		ReviewCallbackPrefix + "reroll:1:-5",
		ReviewCallbackPrefix + "drop:1:2",
		ReviewCallbackPrefix + "show:1:2",
	} {
		if a, ok := parseReviewData(data); ok {
			t.Errorf("parse %q = %+v, want rejected", data, a)
		}
	}
}

func TestParseResultsLink(t *testing.T) {
	chat, post, ok := parsePostPayload("/start results_1234567890_42", ReviewLinkPrefix)
	if !ok || chat != drawChannel || post != drawPost {
		t.Errorf("parse = %d, %d, %v; want the channel and post 42", chat, post, ok)
	}

	if _, _, ok := parsePostPayload("/start results_x_42", ReviewLinkPrefix); ok {
		t.Error("malformed link parsed")
	}

	if got := resultsLink(
		"read_wine_bot",
		drawChannel,
		drawPost,
	); got != "https://t.me/read_wine_bot?start=results_1234567890_42" {
		t.Errorf("resultsLink = %q", got)
	}
}

func TestPublishRacingAnotherAdminsRerollIsStale(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann(), bob()}, cid())
	rv := newTestReviewer(t, store)
	tg := newAdmins()

	// Admin B's reroll lands after admin A's publish passed its own check and before the claim.
	store.onClaim = func() {
		store.onClaim = nil
		rv.Reroll(t.Context(), tg, otherAdmin, reviewID, bob().TelegramID)
	}

	if r := rv.Publish(t.Context(), tg, creatorID, reviewID, 0); !r.Stale {
		t.Errorf("publish = %+v, want stale", r)
	}

	if len(tg.sent) != 0 || !store.pending(t) {
		t.Errorf(
			"sent %d messages, pending %v; want nothing posted and the results pending",
			len(tg.sent),
			store.pending(t),
		)
	}
}

func TestUserLinkPrefersUsername(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		u    user.User
		want string
	}{
		{"username", user.User{TelegramID: 1, FirstName: "Ann", Username: "ann"}, `<a href="https://t.me/ann">Ann</a>`},
		{"no username", user.User{TelegramID: 2, FirstName: "Bob"}, `<a href="tg://user?id=2">Bob</a>`},
	}

	for _, tt := range tests {
		if got := userLink(tt.u, maxMessageLen); got != tt.want {
			t.Errorf("%s: userLink = %q, want %q", tt.name, got, tt.want)
		}
	}
}
