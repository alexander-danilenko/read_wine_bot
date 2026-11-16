package giveaway_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

const otherAdminID int64 = 7_000_000_003

// appFields returns a valid create form for post 42 of channelID, ending a day after s.now.
func appFields(s *setup) giveaway.Fields {
	return giveaway.Fields{
		PostLink:   "https://t.me/c/1234567890/42",
		EndsAt:     s.now.Add(24 * time.Hour).Format(giveaway.AppTimeLayout),
		Winners:    2,
		ButtonText: "Join",
	}
}

// mustCreate creates a giveaway from f as userID and returns it.
func mustCreate(t *testing.T, s *setup, app *giveaway.App, userID int64, f giveaway.Fields) giveaway.View {
	t.Helper()

	v, err := app.Create(t.Context(), s.tg, userID, "en", f)
	if err != nil {
		t.Fatal(err)
	}

	return v
}

func wantAppError(t *testing.T, err error, code, field string) {
	t.Helper()

	var appErr *giveaway.AppError
	if !errors.As(err, &appErr) || appErr.Code != code || appErr.Field != field {
		t.Fatalf("err = %v, want %s on %q", err, code, field)
	}
}

func TestAppCreate(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	v := mustCreate(t, s, app, adminID, appFields(s))

	if v.ID == 0 || v.Status != giveaway.StatusActive || v.Winners != 2 || v.ButtonText != "Join" {
		t.Errorf("view = %+v, want an active giveaway with a real ID", v)
	}

	if v.EndsAt != appFields(s).EndsAt {
		t.Errorf("EndsAt = %q, want the Kyiv time sent, %q", v.EndsAt, appFields(s).EndsAt)
	}

	if s.tg.attached != 1 || s.tg.label != "Join" || s.tg.attachedTo != 42 {
		t.Errorf("attached %d times, label %q, post %d", s.tg.attached, s.tg.label, s.tg.attachedTo)
	}

	if _, err := s.store.Draft(t.Context(), adminID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("draft left after create: %v", err)
	}
}

func TestAppCreateRejects(t *testing.T) {
	tests := []struct {
		name      string
		change    func(s *setup, f *giveaway.Fields)
		wantCode  string
		wantField string
	}{
		{
			name:      "not an admin of the channel",
			change:    func(s *setup, _ *giveaway.Fields) { s.tg.role = giveaway.RolePartner },
			wantCode:  giveaway.CodeNotAdmin,
			wantField: giveaway.FieldPostLink,
		},
		{
			name:      "bot lacks rights",
			change:    func(s *setup, _ *giveaway.Fields) { s.tg.noEdit = true },
			wantCode:  giveaway.CodeBotNoRights,
			wantField: giveaway.FieldPostLink,
		},
		{
			name:      "post in another channel",
			change:    func(s *setup, _ *giveaway.Fields) { s.tg.resolveTo = otherChannelID },
			wantCode:  giveaway.CodeNotPrimaryChannel,
			wantField: giveaway.FieldPostLink,
		},
		{
			name:      "not a post link",
			change:    func(_ *setup, f *giveaway.Fields) { f.PostLink = "https://example.com/42" },
			wantCode:  giveaway.CodeInvalidPostLink,
			wantField: giveaway.FieldPostLink,
		},
		{
			name:      "link to a group",
			change:    func(s *setup, _ *giveaway.Fields) { s.tg.resolveErr = giveaway.ErrNotChannel },
			wantCode:  giveaway.CodeInvalidPostLink,
			wantField: giveaway.FieldPostLink,
		},
		{
			name:      "end in the past",
			change:    func(s *setup, f *giveaway.Fields) { f.EndsAt = s.now.Add(-time.Minute).Format(giveaway.AppTimeLayout) },
			wantCode:  giveaway.CodeEndInPast,
			wantField: giveaway.FieldEndsAt,
		},
		{
			name:      "end not a time",
			change:    func(_ *setup, f *giveaway.Fields) { f.EndsAt = "tomorrow" },
			wantCode:  giveaway.CodeInvalidEnd,
			wantField: giveaway.FieldEndsAt,
		},
		{
			name:      "0 winners",
			change:    func(_ *setup, f *giveaway.Fields) { f.Winners = 0 },
			wantCode:  giveaway.CodeInvalidWinners,
			wantField: giveaway.FieldWinners,
		},
		{
			name:      "99 winners",
			change:    func(_ *setup, f *giveaway.Fields) { f.Winners = 99 },
			wantCode:  giveaway.CodeInvalidWinners,
			wantField: giveaway.FieldWinners,
		},
		{
			name:      "blank button text",
			change:    func(_ *setup, f *giveaway.Fields) { f.ButtonText = "  " },
			wantCode:  giveaway.CodeInvalidButtonText,
			wantField: giveaway.FieldButtonText,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newSetup(t)
			app := giveaway.NewApp(s.ed, nil, s.store, channelID)
			f := appFields(s)
			tc.change(s, &f)

			_, err := app.Create(t.Context(), s.tg, adminID, "en", f)
			wantAppError(t, err, tc.wantCode, tc.wantField)

			if s.tg.attached != 0 || len(s.store.rows) != 0 {
				t.Errorf("attached %d, rows %d, want nothing changed", s.tg.attached, len(s.store.rows))
			}
		})
	}
}

func TestAppCreateOnTakenPost(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	mustCreate(t, s, app, adminID, appFields(s))

	_, err := app.Create(t.Context(), s.tg, adminID, "en", appFields(s))
	wantAppError(t, err, giveaway.CodeExists, giveaway.FieldPostLink)

	_, err = app.Create(t.Context(), s.tg, otherAdminID, "en", appFields(s))
	wantAppError(t, err, giveaway.CodeTaken, giveaway.FieldPostLink)
}

// A failed button step deletes the draft, so the one-draft-per-admin index never blocks the
// admin's next create, on the same post or another.
func TestAppCreateAfterFailedAttach(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	s.tg.attachErr = giveaway.ErrPostGone

	_, err := app.Create(t.Context(), s.tg, adminID, "en", appFields(s))
	wantAppError(t, err, giveaway.CodePostGone, giveaway.FieldPostLink)

	if len(s.store.rows) != 0 {
		t.Errorf("rows = %d after a failed create, want 0", len(s.store.rows))
	}

	s.tg.attachErr = nil
	other := appFields(s)
	other.PostLink = "https://t.me/c/1234567890/43"

	if _, err := app.Create(t.Context(), s.tg, adminID, "en", other); err != nil {
		t.Errorf("create on another post after a failure: %v", err)
	}

	if _, err := app.Create(t.Context(), s.tg, adminID, "en", appFields(s)); err != nil {
		t.Errorf("create on the same post after a failure: %v", err)
	}
}

// AE5: the end is Kyiv wall-clock time whatever the phone's zone; the app sends it as typed.
func TestAppCreateEndIsKyivTime(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	f := appFields(s)
	f.EndsAt = "2026-12-01T20:00"

	v := mustCreate(t, s, app, adminID, f)

	g, err := s.store.ByID(t.Context(), v.ID)
	if err != nil {
		t.Fatal(err)
	}

	if want := time.Date(2026, 12, 1, 18, 0, 0, 0, time.UTC); !g.EndsAt.Equal(want) {
		t.Errorf("stored end = %v, want 20:00 Kyiv = %v", g.EndsAt, want)
	}

	if v.EndsAt != "2026-12-01T20:00" {
		t.Errorf("view end = %q, want 2026-12-01T20:00", v.EndsAt)
	}
}

// Every primary admin gets any giveaway of the primary channel; a giveaway of another
// channel, a draft and a missing ID all answer not_found, even to the creator.
func TestAppGet(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	v := mustCreate(t, s, app, adminID, appFields(s))

	finished := s.store.rows[v.ID]
	finished.FinishedAt = &s.now

	got, err := app.Get(t.Context(), s.tg, otherAdminID, v.ID)
	if err != nil || got.ID != v.ID || got.Status != giveaway.StatusFinished {
		t.Errorf("another admin's Get = %+v, %v, want the finished giveaway", got, err)
	}

	elsewhere := publish(t, s.store, &giveaway.Giveaway{ChatID: otherChannelID, MessageID: 42, CreatedBy: adminID})

	_, err = app.Get(t.Context(), s.tg, adminID, elsewhere.ID)
	wantAppError(t, err, giveaway.CodeNotFound, "")

	text := "Take part"

	_, err = app.Edit(t.Context(), s.tg, adminID, elsewhere.ID, giveaway.Changes{ButtonText: &text})
	wantAppError(t, err, giveaway.CodeNotFound, "")

	err = s.store.SaveDraft(
		t.Context(),
		&giveaway.Giveaway{ChatID: channelID, MessageID: 50, CreatedBy: adminID, Winners: 1},
	)
	if err != nil {
		t.Fatal(err)
	}

	draft, err := s.store.Draft(t.Context(), adminID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = app.Get(t.Context(), s.tg, adminID, draft.ID)
	wantAppError(t, err, giveaway.CodeNotFound, "")

	_, err = app.Get(t.Context(), s.tg, adminID, 999)
	wantAppError(t, err, giveaway.CodeNotFound, "")
}

func TestAppEdit(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	v := mustCreate(t, s, app, adminID, appFields(s))

	text := "Take part"

	_, err := app.Edit(t.Context(), s.tg, otherAdminID, v.ID, giveaway.Changes{ButtonText: &text})
	wantAppError(t, err, giveaway.CodeNotYours, "")

	got, err := app.Edit(t.Context(), s.tg, adminID, v.ID, giveaway.Changes{ButtonText: &text})
	if err != nil {
		t.Fatal(err)
	}

	if got.ButtonText != text || s.tg.label != text {
		t.Errorf("button text %q on the post %q, want %q on both", got.ButtonText, s.tg.label, text)
	}

	tooMany := 99

	_, err = app.Edit(t.Context(), s.tg, adminID, v.ID, giveaway.Changes{Winners: &tooMany})
	wantAppError(t, err, giveaway.CodeInvalidWinners, giveaway.FieldWinners)

	s.store.rows[v.ID].FinishedAt = &s.now

	_, err = app.Edit(t.Context(), s.tg, adminID, v.ID, giveaway.Changes{ButtonText: &text})
	wantAppError(t, err, giveaway.CodeFinished, "")
}

func TestAppCancelAndRestore(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	v := mustCreate(t, s, app, adminID, appFields(s))

	cancelled, err := app.Cancel(t.Context(), s.tg, adminID, v.ID)
	if err != nil {
		t.Fatal(err)
	}

	if cancelled.Status != giveaway.StatusCancelled || s.tg.removed != 1 {
		t.Errorf("status %q, buttons removed %d, want cancelled and 1", cancelled.Status, s.tg.removed)
	}

	// The cancelled giveaway leaves the running list; the creator finds it in the archive.
	wantLists(t, s, app, nil, []uint{v.ID})

	_, err = app.Cancel(t.Context(), s.tg, adminID, v.ID)
	wantAppError(t, err, giveaway.CodeStale, "")

	restored, err := app.Restore(t.Context(), s.tg, adminID, v.ID)
	if err != nil {
		t.Fatal(err)
	}

	if restored.Status != giveaway.StatusActive {
		t.Errorf("status after restore = %q, want active", restored.Status)
	}

	wantLists(t, s, app, []uint{v.ID}, nil)
}

// wantLists checks the IDs on the admin's first running and archive pages.
func wantLists(t *testing.T, s *setup, app *giveaway.App, running, archived []uint) {
	t.Helper()

	r, _, err := app.List(t.Context(), s.tg, adminID, 0, 10)
	if err != nil || !slices.Equal(pageIDs(r), running) || r.Total != len(running) {
		t.Errorf("running = %+v, err %v, want %v", r, err, running)
	}

	a, err := app.Archived(t.Context(), s.tg, adminID, 0, 10)
	if err != nil || !slices.Equal(pageIDs(a), archived) || a.Total != len(archived) {
		t.Errorf("archive = %+v, err %v, want %v", a, err, archived)
	}
}

func TestAppFinishNow(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	v := mustCreate(t, s, app, adminID, appFields(s))

	got, err := app.FinishNow(t.Context(), s.tg, adminID, v.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Status != giveaway.StatusDue {
		t.Errorf("status = %q, want due", got.Status)
	}
}

func TestAppMove(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	v := mustCreate(t, s, app, adminID, appFields(s))

	moved, err := app.Move(t.Context(), s.tg, adminID, v.ID, "https://t.me/c/1234567890/50")
	if err != nil {
		t.Fatal(err)
	}

	if moved.MessageID != 50 || s.tg.attachedTo != 50 || s.tg.removedFrom != 42 {
		t.Errorf("moved to %d, attached to %d, removed from %d", moved.MessageID, s.tg.attachedTo, s.tg.removedFrom)
	}

	other := appFields(s)
	other.PostLink = "https://t.me/c/1234567890/60"

	mustCreate(t, s, app, adminID, other)

	_, err = app.Move(t.Context(), s.tg, adminID, v.ID, other.PostLink)
	wantAppError(t, err, giveaway.CodeTaken, giveaway.FieldPostLink)

	_, err = app.Move(t.Context(), s.tg, adminID, v.ID, "not a link")
	wantAppError(t, err, giveaway.CodeInvalidPostLink, giveaway.FieldPostLink)
}

// Every primary admin lists every giveaway of the primary channel, whoever created it, and
// none of another channel.
func TestAppListCoversPrimaryChannel(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	byA := mustCreate(t, s, app, adminID, appFields(s))

	other := appFields(s)
	other.PostLink = "https://t.me/c/1234567890/43"
	byB := mustCreate(t, s, app, otherAdminID, other)

	elsewhere := publish(t, s.store, &giveaway.Giveaway{ChatID: otherChannelID, MessageID: 42, CreatedBy: otherAdminID})

	running, review, err := app.List(t.Context(), s.tg, otherAdminID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}

	if want := []uint{byB.ID, byA.ID}; !slices.Equal(pageIDs(running), want) || running.Total != 2 || len(review) != 0 {
		t.Fatalf("B's list = %+v, review %+v, want %v", running, review, want)
	}

	// After the draw A's giveaway waits for review: B sees it too. The other channel's stays hidden.
	for _, id := range []uint{byA.ID, elsewhere.ID} {
		s.store.rows[id].FinishedAt, s.store.rows[id].ResultsPending = &s.now, true
	}

	_, review, err = app.List(t.Context(), s.tg, otherAdminID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(review) != 1 || review[0].ID != byA.ID || review[0].Status != giveaway.StatusReview {
		t.Errorf("review = %+v, want only A's drawn giveaway", review)
	}
}

func TestAppArchived(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	ids := make([]uint, 0, 3)

	for _, post := range []string{"42", "43", "44"} {
		f := appFields(s)
		f.PostLink = "https://t.me/c/1234567890/" + post
		ids = append(ids, mustCreate(t, s, app, adminID, f).ID)
	}

	// Drawn an hour ago, cancelled now, drawn two hours ago: the archive orders them by when
	// each left the running list. It holds the primary channel's giveaways whoever created
	// them, and none of another channel.
	early, late := s.now.Add(-2*time.Hour), s.now.Add(-time.Hour)
	s.store.rows[ids[0]].FinishedAt = &late
	s.store.rows[ids[1]].Published, s.store.rows[ids[1]].CancelledAt = false, &s.now
	s.store.rows[ids[2]].FinishedAt, s.store.rows[ids[2]].CreatedBy = &early, otherAdminID

	elsewhere := publish(t, s.store, &giveaway.Giveaway{ChatID: otherChannelID, MessageID: 42, CreatedBy: adminID})
	s.store.rows[elsewhere.ID].FinishedAt = &s.now

	for _, tc := range []struct {
		offset int
		want   []uint
	}{
		{offset: 0, want: []uint{ids[1], ids[0]}}, // the cancelled, then the latest drawn
		{offset: 2, want: []uint{ids[2]}},
		{offset: 10, want: nil}, // past the end
	} {
		page, err := app.Archived(t.Context(), s.tg, adminID, tc.offset, 2)
		if err != nil || page.Total != 3 || !slices.Equal(pageIDs(page), tc.want) {
			t.Errorf("offset %d: page %+v, err %v, want %v of 3", tc.offset, page, err, tc.want)
		}
	}
}

// pageIDs lists the IDs of the giveaways on p, in order.
func pageIDs(p giveaway.Page) []uint {
	ids := make([]uint, 0, len(p.Views))
	for _, v := range p.Views {
		ids = append(ids, v.ID)
	}

	return ids
}

func TestAppListNewestFirst(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)

	first := mustCreate(t, s, app, adminID, appFields(s))

	later := appFields(s)
	later.PostLink = "https://t.me/c/1234567890/43"
	second := mustCreate(t, s, app, adminID, later)

	mine, _, err := app.List(t.Context(), s.tg, adminID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(mine.Views) != 2 || mine.Views[0].ID != second.ID || mine.Views[1].ID != first.ID {
		t.Errorf("list = %+v, want the latest created first", mine)
	}
}

// Channels reads each channel of the lists once, live; a channel Telegram cannot describe
// stays in the result without title or photo.
func TestAppChannels(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	v := mustCreate(t, s, app, adminID, appFields(s))

	got := app.Channels(t.Context(), s.tg, []giveaway.View{v, v}, []giveaway.View{v})
	want := giveaway.Channel{ID: channelID, Title: "Read Wine", PhotoID: "photo-1"}

	if !reflect.DeepEqual(got, []giveaway.Channel{want}) {
		t.Fatalf("channels = %+v, want only %+v", got, want)
	}

	s.tg.channelErr = errBoom

	if got := app.Channels(
		t.Context(),
		s.tg,
		[]giveaway.View{v},
	); !reflect.DeepEqual(got, []giveaway.Channel{{ID: channelID}}) {
		t.Errorf("channels = %+v, want the channel without title or photo", got)
	}
}

// memberLookup answers ChatMember with m or err and records what it was asked.
type memberLookup struct {
	m              models.ChatMember
	err            error
	chatID, userID int64
}

func (f *memberLookup) ChatMember(_ context.Context, chatID, userID int64) (models.ChatMember, error) {
	f.chatID, f.userID = chatID, userID

	return f.m, f.err
}

func TestAppRole(t *testing.T) {
	owner := models.ChatMember{Type: models.ChatMemberTypeOwner, Owner: &models.ChatMemberOwner{}}
	left := models.ChatMember{Type: models.ChatMemberTypeLeft, Left: &models.ChatMemberLeft{}}

	tests := []struct {
		name    string
		tg      *memberLookup
		want    giveaway.Role
		wantErr bool
	}{
		{"owner of the primary channel", &memberLookup{m: owner}, giveaway.RoleFull, false},
		{"not in the primary channel", &memberLookup{m: left}, giveaway.RolePartner, false},
		{"bot lost the channel (403)", &memberLookup{err: fmt.Errorf("%w, kicked", bot.ErrorForbidden)}, "", true},
		{
			"bot lost the channel (400)",
			&memberLookup{err: fmt.Errorf("%w, chat not found", bot.ErrorBadRequest)},
			"",
			true,
		},
		{"network error", &memberLookup{err: errors.New("connection reset")}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			app := giveaway.NewApp(s.ed, nil, s.store, channelID)

			got, err := app.Role(t.Context(), tt.tg, adminID)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("Role() = %q, %v; want %q, error %v", got, err, tt.want, tt.wantErr)
			}

			if tt.tg.chatID != channelID || tt.tg.userID != adminID {
				t.Errorf("looked up user %d in chat %d, want user %d in the primary channel %d",
					tt.tg.userID, tt.tg.chatID, adminID, channelID)
			}
		})
	}
}

// An admin without Post messages or Edit messages of others cannot create a giveaway, even one
// who had the rights when they opened the form: the role is read again on submit, and the
// refusal comes before anything reaches the store or the post.
func TestAppCreateByReadOnlyAdmin(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	s.tg.role = giveaway.RoleReadOnly

	_, err := app.Create(t.Context(), s.tg, adminID, "en", appFields(s))
	wantAppError(t, err, giveaway.CodeReadOnly, "")

	if s.store.reads != 0 || len(s.store.rows) != 0 || s.tg.attached != 0 {
		t.Errorf("store reads %d, rows %d, attached %d; want nothing touched",
			s.store.reads, len(s.store.rows), s.tg.attached)
	}
}

// A creator who lost a right gets read_only for every change of their own giveaway, and
// nothing changes.
func TestAppChangesByReadOnlyCreator(t *testing.T) {
	text := "Take part"

	for name, change := range map[string]func(*setup, *giveaway.App, uint) (giveaway.View, error){
		"edit": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Edit(t.Context(), s.tg, adminID, id, giveaway.Changes{ButtonText: &text})
		},
		"move": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Move(t.Context(), s.tg, adminID, id, "https://t.me/c/1234567890/50")
		},
		"cancel": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Cancel(t.Context(), s.tg, adminID, id)
		},
		"restore": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Restore(t.Context(), s.tg, adminID, id)
		},
		"finish now": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.FinishNow(t.Context(), s.tg, adminID, id)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSetup(t)
			app := giveaway.NewApp(s.ed, nil, s.store, channelID)
			v := mustCreate(t, s, app, adminID, appFields(s))
			before := *s.store.rows[v.ID]
			s.tg.role = giveaway.RoleReadOnly

			_, err := change(s, app, v.ID)
			wantAppError(t, err, giveaway.CodeReadOnly, "")

			if after := *s.store.rows[v.ID]; !reflect.DeepEqual(after, before) || s.tg.removed != 0 {
				t.Errorf("giveaway changed: %+v, buttons removed %d", after, s.tg.removed)
			}
		})
	}
}

// Only admins of the primary channel see its giveaways: a partner gets not_admin for the
// lists and the details, and the store is never read for them.
func TestAppReadsByPartner(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	v := mustCreate(t, s, app, adminID, appFields(s))
	s.tg.role, s.store.reads = giveaway.RolePartner, 0

	_, _, err := app.List(t.Context(), s.tg, fanID, 0, 10)
	wantAppError(t, err, giveaway.CodeNotAdmin, "")

	_, err = app.Archived(t.Context(), s.tg, fanID, 0, 10)
	wantAppError(t, err, giveaway.CodeNotAdmin, "")

	_, err = app.Get(t.Context(), s.tg, fanID, v.ID)
	wantAppError(t, err, giveaway.CodeNotAdmin, "")

	if s.store.reads != 0 {
		t.Errorf("store read %d times for a partner, want 0", s.store.reads)
	}
}

// A failed role lookup fails the request: it never answers as for a partner (not_admin) or a
// read-only admin, and a change or a create leaves everything as it was.
func TestAppFailedRoleLookup(t *testing.T) {
	errTelegram := errors.New("telegram is down")
	text := "Take part"

	reads := map[string]func(*setup, *giveaway.App, uint) error{
		"list": func(s *setup, app *giveaway.App, _ uint) error {
			_, _, err := app.List(t.Context(), s.tg, adminID, 0, 10)
			return err
		},
		"archived": func(s *setup, app *giveaway.App, _ uint) error {
			_, err := app.Archived(t.Context(), s.tg, adminID, 0, 10)
			return err
		},
		"get": func(s *setup, app *giveaway.App, id uint) error {
			_, err := app.Get(t.Context(), s.tg, adminID, id)
			return err
		},
	}

	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			s := newSetup(t)
			app := giveaway.NewApp(s.ed, nil, s.store, channelID)
			v := mustCreate(t, s, app, adminID, appFields(s))
			s.tg.roleErr = errTelegram

			err := read(s, app, v.ID)

			var appErr *giveaway.AppError
			if err == nil || errors.As(err, &appErr) {
				t.Errorf("err = %v, want a plain error, not an AppError", err)
			}
		})
	}

	t.Run("create", func(t *testing.T) {
		s := newSetup(t)
		app := giveaway.NewApp(s.ed, nil, s.store, channelID)
		s.tg.roleErr = errTelegram

		_, err := app.Create(t.Context(), s.tg, adminID, "en", appFields(s))
		wantAppError(t, err, giveaway.CodeFailed, "")

		if len(s.store.rows) != 0 || s.tg.attached != 0 {
			t.Errorf("rows %d, attached %d; want nothing saved or attached", len(s.store.rows), s.tg.attached)
		}
	})

	changes := map[string]func(*setup, *giveaway.App, uint) (giveaway.View, error){
		"edit": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Edit(t.Context(), s.tg, adminID, id, giveaway.Changes{ButtonText: &text})
		},
		"move": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Move(t.Context(), s.tg, adminID, id, "https://t.me/c/1234567890/50")
		},
		"cancel": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Cancel(t.Context(), s.tg, adminID, id)
		},
		"restore": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.Restore(t.Context(), s.tg, adminID, id)
		},
		"finish now": func(s *setup, app *giveaway.App, id uint) (giveaway.View, error) {
			return app.FinishNow(t.Context(), s.tg, adminID, id)
		},
	}

	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s := newSetup(t)
			app := giveaway.NewApp(s.ed, nil, s.store, channelID)
			v := mustCreate(t, s, app, adminID, appFields(s))
			before := *s.store.rows[v.ID]
			s.tg.roleErr = errTelegram

			_, err := change(s, app, v.ID)
			wantAppError(t, err, giveaway.CodeFailed, "")

			if after := *s.store.rows[v.ID]; !reflect.DeepEqual(after, before) || s.tg.removed != 0 {
				t.Errorf("giveaway changed: %+v, buttons removed %d", after, s.tg.removed)
			}
		})
	}
}

// A giveaway's view says whether the caller may change it: only its creator, while a full
// admin. Another full admin and a read-only creator see it unchangeable, on the details and
// in the list.
func TestAppCanChange(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	v := mustCreate(t, s, app, adminID, appFields(s))

	if !v.CanChange {
		t.Error("the creator's new giveaway: can_change false, want true")
	}

	for _, tc := range []struct {
		name   string
		role   giveaway.Role
		userID int64
		want   bool
	}{
		{"creator", giveaway.RoleFull, adminID, true},
		{"another full admin", giveaway.RoleFull, otherAdminID, false},
		{"read-only creator", giveaway.RoleReadOnly, adminID, false},
	} {
		s.tg.role = tc.role

		got, err := app.Get(t.Context(), s.tg, tc.userID, v.ID)
		if err != nil || got.CanChange != tc.want {
			t.Errorf("%s: details can_change %v, err %v; want %v", tc.name, got.CanChange, err, tc.want)
		}

		page, _, err := app.List(t.Context(), s.tg, tc.userID, 0, 10)
		if err != nil || len(page.Views) != 1 || page.Views[0].CanChange != tc.want {
			t.Errorf("%s: list %+v, err %v; want can_change %v", tc.name, page.Views, err, tc.want)
		}
	}
}
