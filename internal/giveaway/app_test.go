package giveaway_test

import (
	"errors"
	"testing"
	"time"

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
	app := giveaway.NewApp(s.ed, s.store)

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
			change:    func(s *setup, _ *giveaway.Fields) { s.tg.admin = false },
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
			app := giveaway.NewApp(s.ed, s.store)
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
	app := giveaway.NewApp(s.ed, s.store)

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
	app := giveaway.NewApp(s.ed, s.store)

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
	app := giveaway.NewApp(s.ed, s.store)
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

func TestAppGetOnlyForCreator(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, s.store)

	v := mustCreate(t, s, app, adminID, appFields(s))

	_, err := app.Get(t.Context(), otherAdminID, v.ID)
	wantAppError(t, err, giveaway.CodeNotFound, "")

	finished := s.store.rows[v.ID]
	finished.FinishedAt = &s.now

	_, err = app.Get(t.Context(), otherAdminID, v.ID)
	wantAppError(t, err, giveaway.CodeNotFound, "")

	got, err := app.Get(t.Context(), adminID, v.ID)
	if err != nil || got.Status != giveaway.StatusFinished {
		t.Errorf("creator Get = %+v, %v, want the finished giveaway", got, err)
	}

	_, err = app.Get(t.Context(), adminID, 999)
	wantAppError(t, err, giveaway.CodeNotFound, "")
}

func TestAppEdit(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, s.store)

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
	app := giveaway.NewApp(s.ed, s.store)

	v := mustCreate(t, s, app, adminID, appFields(s))

	cancelled, err := app.Cancel(t.Context(), s.tg, adminID, v.ID)
	if err != nil {
		t.Fatal(err)
	}

	if cancelled.Status != giveaway.StatusCancelled || s.tg.removed != 1 {
		t.Errorf("status %q, buttons removed %d, want cancelled and 1", cancelled.Status, s.tg.removed)
	}

	// The creator can still find the cancelled giveaway, to restore it.
	mine, _, err := app.List(t.Context(), s.tg, adminID)
	if err != nil {
		t.Fatal(err)
	}

	if len(mine) != 1 || mine[0].Status != giveaway.StatusCancelled {
		t.Fatalf("list = %+v, want the cancelled giveaway", mine)
	}

	_, err = app.Cancel(t.Context(), s.tg, adminID, v.ID)
	wantAppError(t, err, giveaway.CodeStale, "")

	restored, err := app.Restore(t.Context(), s.tg, adminID, v.ID)
	if err != nil {
		t.Fatal(err)
	}

	if restored.Status != giveaway.StatusActive {
		t.Errorf("status after restore = %q, want active", restored.Status)
	}
}

func TestAppFinishNow(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, s.store)

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
	app := giveaway.NewApp(s.ed, s.store)

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

func TestAppListAndFinished(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, s.store)

	v := mustCreate(t, s, app, adminID, appFields(s))

	other := appFields(s)
	other.PostLink = "https://t.me/c/1234567890/43"

	mustCreate(t, s, app, otherAdminID, other)

	mine, review, err := app.List(t.Context(), s.tg, adminID)
	if err != nil {
		t.Fatal(err)
	}

	if len(mine) != 1 || mine[0].ID != v.ID || len(review) != 0 {
		t.Fatalf("list = %+v, review %+v, want only the admin's giveaway", mine, review)
	}

	// After the draw the other admin's giveaway waits for review: every channel admin sees it.
	drawn := s.store.rows[v.ID+1]
	drawn.FinishedAt, drawn.ResultsPending = &s.now, true

	_, review, err = app.List(t.Context(), s.tg, adminID)
	if err != nil {
		t.Fatal(err)
	}

	if len(review) != 1 || review[0].Status != giveaway.StatusReview {
		t.Errorf("review = %+v, want the drawn giveaway", review)
	}
}

func TestAppFinished(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, s.store)

	for _, post := range []string{"42", "43", "44"} {
		f := appFields(s)
		f.PostLink = "https://t.me/c/1234567890/" + post
		v := mustCreate(t, s, app, adminID, f)
		s.store.rows[v.ID].FinishedAt = &s.now
	}

	page, more, err := app.Finished(t.Context(), adminID, 0, 2)
	if err != nil || len(page) != 2 || !more {
		t.Errorf("first page = %+v, more %v, err %v, want 2 and more", page, more, err)
	}

	page, more, err = app.Finished(t.Context(), adminID, 2, 2)
	if err != nil || len(page) != 1 || more {
		t.Errorf("last page = %+v, more %v, err %v, want 1 and no more", page, more, err)
	}
}
