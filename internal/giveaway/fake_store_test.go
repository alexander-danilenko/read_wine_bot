package giveaway_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

const (
	channelID int64 = -1_001_234_567_890 // channel IDs are negative and larger than int32
	adminID   int64 = 7_000_000_001
	fanID     int64 = 7_000_000_002
)

// memStore keeps giveaways in memory and follows the contract documented on *giveaway.Store:
// a draft is unpublished without a cancel mark, and only published or cancelled giveaways
// are managed.
type memStore struct {
	mu           sync.Mutex
	rows         map[uint]*giveaway.Giveaway
	participants map[uint]map[int64]bool
	winners      map[uint]int // current winner count per giveaway, set by tests
	nextID       uint
}

// errConstraint stands in for a database constraint violation.
var errConstraint = errors.New("constraint violated")

func newMemStore() *memStore {
	return &memStore{
		rows: map[uint]*giveaway.Giveaway{}, participants: map[uint]map[int64]bool{}, winners: map[uint]int{},
	}
}

func isDraft(g *giveaway.Giveaway) bool { return !g.Published && g.CancelledAt == nil }

func isManaged(g *giveaway.Giveaway) bool { return g.Published || g.CancelledAt != nil }

func (f *memStore) Publish(_ context.Context, id uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	g, ok := f.rows[id]
	if !ok || g.Published {
		return giveaway.ErrNotFound
	}

	g.Published, g.CancelledAt = true, nil

	return nil
}

func (f *memStore) SaveDraft(_ context.Context, g *giveaway.Giveaway) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.violates(g) {
		return errConstraint
	}

	now := time.Now()

	for _, id := range f.ids() {
		row := f.rows[id]
		if row.ChatID != g.ChatID || row.MessageID != g.MessageID {
			continue
		}

		if !isDraft(row) {
			return giveaway.ErrExists
		}

		row.CreatedBy, row.Language, row.EndsAt = g.CreatedBy, g.Language, g.EndsAt
		row.Winners, row.ButtonText, row.UpdatedAt = g.Winners, g.ButtonText, now

		return nil
	}

	f.nextID++
	row := *g
	row.ID, row.Published, row.CancelledAt, row.FinishedAt = f.nextID, false, nil, nil
	row.CreatedAt, row.UpdatedAt = now, now
	f.rows[row.ID] = &row

	return nil
}

func (f *memStore) Draft(_ context.Context, userID int64) (giveaway.Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, id := range f.ids() {
		if g := f.rows[id]; g.CreatedBy == userID && isDraft(g) {
			return *g, nil
		}
	}

	return giveaway.Giveaway{}, giveaway.ErrNotFound
}

func (f *memStore) DeleteDraft(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for id, g := range f.rows {
		if g.CreatedBy == userID && isDraft(g) {
			delete(f.rows, id)
		}
	}

	return nil
}

func (f *memStore) Join(_ context.Context, giveawayID uint, telegramID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.participants[giveawayID] == nil {
		f.participants[giveawayID] = map[int64]bool{}
	}

	if f.participants[giveawayID][telegramID] {
		return false, nil
	}

	f.participants[giveawayID][telegramID] = true

	return true, nil
}

func (f *memStore) Participants(_ context.Context, ids []uint) (map[uint]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	counts := map[uint]int{}

	for _, id := range ids {
		if n := len(f.participants[id]); n > 0 {
			counts[id] = n
		}
	}

	return counts, nil
}

func (f *memStore) Post(_ context.Context, chatID, messageID int64) (giveaway.Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, id := range f.ids() {
		if g := f.rows[id]; g.ChatID == chatID && g.MessageID == messageID && isManaged(g) {
			return *g, nil
		}
	}

	return giveaway.Giveaway{}, giveaway.ErrNotFound
}

func (f *memStore) ByID(_ context.Context, id uint) (giveaway.Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if g, ok := f.rows[id]; ok {
		return *g, nil
	}

	return giveaway.Giveaway{}, giveaway.ErrNotFound
}

// Finished sorts by finish time, latest first, then by ID, latest first, as Store.Finished.
func (f *memStore) Finished(_ context.Context, userID int64, offset, limit int) ([]giveaway.Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var gs []giveaway.Giveaway

	for _, id := range f.ids() {
		if g := f.rows[id]; g.CreatedBy == userID && g.FinishedAt != nil {
			gs = append(gs, *g)
		}
	}

	slices.SortFunc(gs, func(a, b giveaway.Giveaway) int {
		if c := b.FinishedAt.Compare(*a.FinishedAt); c != 0 {
			return c
		}

		return int(b.ID) - int(a.ID)
	})

	gs = gs[min(offset, len(gs)):]

	return gs[:min(limit, len(gs))], nil
}

func (f *memStore) WinnerCounts(_ context.Context, ids []uint) (map[uint]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	counts := map[uint]int{}

	for _, id := range ids {
		if n := f.winners[id]; n > 0 {
			counts[id] = n
		}
	}

	return counts, nil
}

// Running sorts giveaways without an end time last, as Postgres sorts NULLs in ascending order.
func (f *memStore) Running(_ context.Context, userID int64) ([]giveaway.Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var gs []giveaway.Giveaway

	for _, id := range f.ids() {
		if g := f.rows[id]; g.CreatedBy == userID && g.Published && g.FinishedAt == nil {
			gs = append(gs, *g)
		}
	}

	slices.SortStableFunc(gs, func(a, b giveaway.Giveaway) int {
		switch {
		case a.EndsAt == nil && b.EndsAt == nil:
			return 0
		case a.EndsAt == nil:
			return 1
		case b.EndsAt == nil:
			return -1
		default:
			return a.EndsAt.Compare(*b.EndsAt)
		}
	})

	return gs, nil
}

func (f *memStore) SaveFields(_ context.Context, id uint, endsAt *time.Time, winners int, buttonText string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	g, ok := f.rows[id]
	if !ok || g.FinishedAt != nil {
		return giveaway.ErrNotFound
	}

	g.EndsAt, g.Winners, g.ButtonText = endsAt, winners, buttonText

	return nil
}

func (f *memStore) Cancel(_ context.Context, id uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	g, ok := f.rows[id]
	if !ok || !g.Published || g.FinishedAt != nil {
		return giveaway.ErrNotFound
	}

	now := time.Now()
	g.Published, g.CancelledAt = false, &now

	return nil
}

func (f *memStore) Move(_ context.Context, id uint, messageID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	g, ok := f.rows[id]
	if !ok || g.FinishedAt != nil {
		return giveaway.ErrNotFound
	}

	// idx_giveaways_post allows one giveaway, a draft included, per post.
	for _, row := range f.rows {
		if row.ChatID == g.ChatID && row.MessageID == messageID && row.ID != id {
			return giveaway.ErrExists
		}
	}

	g.MessageID = messageID

	return nil
}

// PendingResults follows chk_giveaways_results: only a finished giveaway has pending results.
func (f *memStore) PendingResults(context.Context) ([]giveaway.Giveaway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var gs []giveaway.Giveaway

	for _, id := range f.ids() {
		if g := f.rows[id]; g.ResultsPending && g.FinishedAt != nil {
			gs = append(gs, *g)
		}
	}

	return gs, nil
}

// ids returns the row IDs in ascending order, so lookups are deterministic.
// violates reports whether the database would reject g as a draft: chk_giveaways_winners
// needs a winner, and idx_giveaways_draft allows one draft per admin.
func (f *memStore) violates(g *giveaway.Giveaway) bool {
	if g.Winners < 1 {
		return true
	}

	return slices.ContainsFunc(f.ids(), func(id uint) bool {
		row := f.rows[id]

		return row.CreatedBy == g.CreatedBy && isDraft(row) && (row.ChatID != g.ChatID || row.MessageID != g.MessageID)
	})
}

func (f *memStore) ids() []uint {
	ids := make([]uint, 0, len(f.rows))
	for id := range f.rows {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	return ids
}

// finish marks giveaway id finished, as the draw does.
func (f *memStore) finish(t *testing.T, id uint) {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	g, ok := f.rows[id]
	if !ok {
		t.Fatalf("finish: no giveaway %d", id)
	}

	now := time.Now()
	g.FinishedAt = &now
}

// draw marks giveaway id finished with its results waiting for review, as the draw does.
func (f *memStore) draw(t *testing.T, id uint) {
	t.Helper()

	f.finish(t, id)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.rows[id].ResultsPending = true
}

// fakeUsers keeps each user's language, pending command and keyboard message in memory.
type fakeUsers struct {
	mu       sync.Mutex
	language map[int64]string
	pending  map[int64]string
	keyboard map[int64]int64
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{language: map[int64]string{}, pending: map[int64]string{}, keyboard: map[int64]int64{}}
}

func (f *fakeUsers) KeyboardMessage(_ context.Context, telegramID int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.keyboard[telegramID], nil
}

func (f *fakeUsers) SetKeyboardMessage(_ context.Context, telegramID, messageID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.keyboard[telegramID] = messageID

	return nil
}

func (f *fakeUsers) Language(_ context.Context, telegramID int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.language[telegramID], nil
}

func (f *fakeUsers) PendingCommand(_ context.Context, telegramID int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.pending[telegramID], nil
}

func (f *fakeUsers) SetPendingCommand(_ context.Context, telegramID int64, command string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pending[telegramID] = command

	return nil
}

func (f *fakeUsers) MovePendingCommand(_ context.Context, telegramID int64, from, to string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.pending[telegramID] != from {
		return false, nil
	}

	f.pending[telegramID] = to

	return true, nil
}

// publish saves g as its creator's draft and publishes it, the way Confirm does.
func publish(t *testing.T, store *memStore, g *giveaway.Giveaway) giveaway.Giveaway {
	t.Helper()

	g.Winners = max(g.Winners, 1) // the database requires a winner; most tests need no count

	ctx := t.Context()
	if err := store.SaveDraft(ctx, g); err != nil {
		t.Fatal(err)
	}

	d, err := store.Draft(ctx, g.CreatedBy)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Publish(ctx, d.ID); err != nil {
		t.Fatal(err)
	}

	d.Published = true

	return d
}
