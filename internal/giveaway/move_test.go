package giveaway_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

const otherChannelID int64 = -1_009_876_543_210

// moveStep is the pending command while the bot waits for the new post of giveaway id.
func moveStep(id uint) string { return "edit:move:" + strconv.FormatUint(uint64(id), 10) }

// askMove taps the move button of giveaway id and checks that the bot now waits for a post.
func (s *setup) askMove(t *testing.T, id uint) {
	t.Helper()

	if r := s.ed.AskMove(t.Context(), s.tg, adminID, id); r.Next != moveStep(id) || r.Notice != "" {
		t.Fatalf("AskMove = %+v, want the move step", r)
	}

	if s.step(t) != moveStep(id) {
		t.Fatalf("step %q, want %q", s.step(t), moveStep(id))
	}
}

// joined opens a published giveaway on post 42 with a participant.
func (s *setup) joined(t *testing.T) giveaway.Giveaway {
	t.Helper()

	g := s.open(t, &giveaway.Giveaway{
		ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go", EndsAt: s.kyivTime(20, 19, 0),
	})

	if _, err := s.store.Join(t.Context(), g.ID, fanID); err != nil {
		t.Fatal(err)
	}

	return g
}

func TestMoveActive(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.joined(t)
	s.askMove(t, g.ID)

	want := giveaway.Reply{Notice: i18n.EditMoved, Next: menu(g.ID)}
	if r := s.ed.Move(ctx, s.tg, s.tg, adminID, channelID, 43); r != want {
		t.Fatalf("Move = %+v, want moved and the menu", r)
	}

	if got := s.read(t, g.ID); got.MessageID != 43 || got.ChatID != channelID || !got.Published {
		t.Errorf("giveaway %+v, want published on post 43 of the same channel", got)
	}

	if s.tg.attachedTo != 43 || s.tg.label != "Go" || s.tg.removedFrom != 42 {
		t.Errorf("attached to %d with %q, removed from %d; want 43 with Go, 42",
			s.tg.attachedTo, s.tg.label, s.tg.removedFrom)
	}

	if added, err := s.store.Join(ctx, g.ID, fanID); err != nil || added {
		t.Errorf("Join after move = %v, %v; want the participant kept", added, err)
	}
}

func TestMoveTwice(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.joined(t)
	s.askMove(t, g.ID)

	if r := s.ed.Move(ctx, s.tg, s.tg, adminID, channelID, 43); r.Notice != i18n.EditMoved {
		t.Fatalf("Move = %+v, want moved", r)
	}

	if r := s.ed.Move(ctx, s.tg, s.tg, adminID, channelID, 44); !r.Stale || s.read(t, g.ID).MessageID != 43 {
		t.Errorf("second Move = %+v, want stale and the giveaway left on post 43", r)
	}
}

func TestMoveCancelled(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.cancelOpen(t)
	attached, removed := s.tg.attached, s.tg.removed
	s.askMove(t, g.ID)

	if r := s.ed.Move(ctx, s.tg, s.tg, adminID, channelID, 43); r.Notice != i18n.EditMoved {
		t.Fatalf("Move = %+v, want moved", r)
	}

	if got := s.read(t, g.ID); got.MessageID != 43 || got.Published {
		t.Errorf("giveaway %+v, want cancelled on post 43", got)
	}

	if s.tg.attached != attached || s.tg.removed != removed {
		t.Errorf("attached %d, removed %d; want no button changes for a cancelled giveaway",
			s.tg.attached-attached, s.tg.removed-removed)
	}

	if r := s.ed.Restore(ctx, s.tg, adminID, g.ID); r.Notice != i18n.EditRestored || s.tg.attachedTo != 43 {
		t.Errorf("Restore = %+v, attached to %d; want restored on post 43", r, s.tg.attachedTo)
	}
}

func TestMoveRejectedPosts(t *testing.T) {
	tests := []struct {
		name      string
		chatID    int64
		messageID int64
		want      i18n.Key
	}{
		{"another channel", otherChannelID, 43, i18n.EditMoveOtherChannel},
		{"post with another giveaway", channelID, 50, i18n.EditMoveTaken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			g := s.joined(t)
			publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 50, CreatedBy: fanID})
			s.askMove(t, g.ID)

			r := s.ed.Move(t.Context(), s.tg, s.tg, adminID, tt.chatID, tt.messageID)
			if r != (giveaway.Reply{Notice: tt.want, Next: moveStep(g.ID)}) {
				t.Errorf("Move = %+v, want %s and the move step", r, tt.want)
			}

			if got := s.read(t, g.ID); got.MessageID != 42 || s.step(t) != moveStep(g.ID) {
				t.Errorf("post %d, step %q; want 42 and the move step kept", got.MessageID, s.step(t))
			}
		})
	}
}

func TestMoveSamePost(t *testing.T) {
	s := newSetup(t)
	g := s.joined(t)
	s.askMove(t, g.ID)

	want := giveaway.Reply{Notice: i18n.EditMoveSame, Next: menu(g.ID)}
	if r := s.ed.Move(t.Context(), s.tg, s.tg, adminID, channelID, 42); r != want || s.step(t) != "" {
		t.Errorf("Move = %+v, step %q; want same-post notice, the menu and no step", r, s.step(t))
	}

	if s.tg.attached != 0 || s.tg.removed != 0 {
		t.Errorf("attached %d, removed %d; want no button changes", s.tg.attached, s.tg.removed)
	}
}

func TestMoveOntoDraft(t *testing.T) {
	s := newSetup(t)
	g := s.joined(t)
	draft := &giveaway.Giveaway{ChatID: channelID, MessageID: 43, CreatedBy: fanID, Winners: 1}
	if err := s.store.SaveDraft(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	s.askMove(t, g.ID)

	r := s.ed.Move(t.Context(), s.tg, s.tg, adminID, channelID, 43)
	if r != (giveaway.Reply{Notice: i18n.EditMoveTaken, Next: moveStep(g.ID)}) {
		t.Errorf("Move = %+v, want taken and the move step", r)
	}

	if got := s.read(t, g.ID); got.MessageID != 42 || s.step(t) != moveStep(g.ID) {
		t.Errorf("post %d, step %q; want 42 and the move step kept", got.MessageID, s.step(t))
	}

	if s.tg.attachedTo != 43 || s.tg.removedFrom != 43 {
		t.Errorf("attached to %d, removed from %d; want the new button taken off post 43 again",
			s.tg.attachedTo, s.tg.removedFrom)
	}
}

func TestMoveAttachFails(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want i18n.Key
	}{
		{"post deleted", giveaway.ErrPostGone, i18n.EditPostGone},
		{"no rights", errBoom, i18n.EditNoRights},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			g := s.joined(t)
			s.askMove(t, g.ID)
			s.tg.attachErr = tt.err

			r := s.ed.Move(t.Context(), s.tg, s.tg, adminID, channelID, 43)
			if r != (giveaway.Reply{Notice: tt.want, Next: moveStep(g.ID)}) {
				t.Errorf("Move = %+v, want %s and the move step", r, tt.want)
			}

			if got := s.read(t, g.ID); got.MessageID != 42 || s.step(t) != moveStep(g.ID) {
				t.Errorf("post %d, step %q; want 42 and the move step kept", got.MessageID, s.step(t))
			}
		})
	}
}

func TestMoveOldButtonStays(t *testing.T) {
	s := newSetup(t)
	g := s.joined(t)
	s.askMove(t, g.ID)
	s.tg.removeErr = errBoom

	if r := s.ed.Move(t.Context(), s.tg, s.tg, adminID, channelID, 43); r.Notice != i18n.EditMoved {
		t.Errorf("Move = %+v, want moved although the old button stayed", r)
	}

	if got := s.read(t, g.ID); got.MessageID != 43 {
		t.Errorf("post %d, want 43", got.MessageID)
	}
}

func TestMoveDue(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.open(
		t,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: s.kyivTime(9, 20, 0)},
	)
	s.askMove(t, g.ID)

	s.now = *s.kyivTime(9, 20, 1)
	if r := s.ed.Move(ctx, s.tg, s.tg, adminID, channelID, 43); r != (giveaway.Reply{Notice: i18n.EditFinishing}) {
		t.Errorf("Move after the end = %+v, want the finishing notice", r)
	}

	if s.step(t) != "" {
		t.Errorf("step %q, want the move step ended", s.step(t))
	}

	if got := s.read(t, g.ID); got.MessageID != 42 || s.tg.attached != 0 {
		t.Errorf("post %d, attached %d; want 42 and nothing attached", got.MessageID, s.tg.attached)
	}

	if r := s.ed.AskMove(ctx, s.tg, adminID, g.ID); !r.Stale {
		t.Errorf("AskMove after the end = %+v, want stale", r)
	}
}

func TestMoveFinished(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.joined(t)

	s.store.finish(t, g.ID)

	if r := s.ed.AskMove(ctx, s.tg, adminID, g.ID); r.Notice != i18n.EditFinished {
		t.Errorf("AskMove = %+v, want the finished notice", r)
	}
}

func TestRestoreDeletedPost(t *testing.T) {
	s := newSetup(t)
	g := s.cancelOpen(t)
	s.tg.attachErr = giveaway.ErrPostGone

	want := giveaway.Reply{Notice: i18n.EditPostGone, Next: menu(g.ID)}
	if r := s.ed.Restore(t.Context(), s.tg, adminID, g.ID); r != want {
		t.Errorf("Restore = %+v, want post-gone and the menu", r)
	}

	if s.read(t, g.ID).Published {
		t.Error("giveaway published although its post is gone")
	}
}

// failingMover fails every move with err.
type failingMover struct{ err error }

func (f failingMover) Move(context.Context, uint, int64) error { return f.err }

func TestMoveStoreFails(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want giveaway.Reply
	}{
		{"finished meanwhile", giveaway.ErrNotFound, giveaway.Reply{Notice: i18n.EditFinished}},
		{"database error", errBoom, giveaway.Reply{Notice: i18n.EditFailed, Next: menu(1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			g := s.joined(t)
			ed := giveaway.NewEditor(s.svc, s.store, s.store, failingMover{tt.err}, s.store, s.store)

			if r := ed.AskMove(t.Context(), s.tg, adminID, g.ID); r.Next != moveStep(g.ID) {
				t.Fatalf("AskMove = %+v, want the move step", r)
			}

			if r := ed.Move(t.Context(), s.tg, s.tg, adminID, channelID, 43); r != tt.want {
				t.Errorf("Move = %+v, want %+v", r, tt.want)
			}

			if s.tg.attachedTo != 43 || s.tg.removedFrom != 43 || s.read(t, g.ID).MessageID != 42 {
				t.Errorf(
					"attached to %d, removed from %d, post %d; want the new button taken off 43 and the giveaway on 42",
					s.tg.attachedTo,
					s.tg.removedFrom,
					s.read(t, g.ID).MessageID,
				)
			}
		})
	}
}

func TestEditButtonTextOnDeletedPost(t *testing.T) {
	s := newSetup(t)
	g := s.joined(t)
	s.tg.attachErr = giveaway.ErrPostGone

	if r := s.change(
		t,
		g.ID,
		giveaway.FieldButtonText,
		"Join",
	); r != (giveaway.Reply{Notice: i18n.EditPostGone, Next: menu(g.ID)}) {
		t.Errorf("button text change = %+v, want post-gone and the menu", r)
	}

	if s.read(t, g.ID).ButtonText != "Go" {
		t.Error("button text saved although the post is gone")
	}
}
