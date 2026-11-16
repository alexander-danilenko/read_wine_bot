package giveaway_test

import (
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// ADR 0006: a move whose new button fails leaves the giveaway on its old post.
func TestAppMoveAttachFails(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	v := mustCreate(t, s, app, adminID, appFields(s))

	s.tg.attachErr = giveaway.ErrPostGone

	_, err := app.Move(t.Context(), s.tg, adminID, v.ID, "https://t.me/c/1234567890/50")
	wantAppError(t, err, giveaway.CodePostGone, "")

	if got := s.store.rows[v.ID].MessageID; got != 42 {
		t.Errorf("MessageID = %d after a failed move, want 42", got)
	}
}

// Cancel removes the button before it writes the row, so a failed removal changes nothing.
func TestAppCancelButtonStays(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	v := mustCreate(t, s, app, adminID, appFields(s))

	s.tg.removeErr = errBoom

	_, err := app.Cancel(t.Context(), s.tg, adminID, v.ID)
	wantAppError(t, err, giveaway.CodeBotNoRights, "")

	if g := s.store.rows[v.ID]; g.CancelledAt != nil || !g.Published {
		t.Errorf("row after a failed cancel = %+v, want still published", g)
	}
}

func TestAppRestoreAfterEnd(t *testing.T) {
	s := newSetup(t)
	app := giveaway.NewApp(s.ed, nil, s.store, channelID)
	v := mustCreate(t, s, app, adminID, appFields(s))

	if _, err := app.Cancel(t.Context(), s.tg, adminID, v.ID); err != nil {
		t.Fatal(err)
	}

	s.now = s.now.Add(48 * time.Hour)

	_, err := app.Restore(t.Context(), s.tg, adminID, v.ID)
	wantAppError(t, err, giveaway.CodeEndInPast, giveaway.FieldEndsAt)
}
