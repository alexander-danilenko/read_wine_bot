package giveaway_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// kyivTime returns the moment in Kyiv as a pointer for Giveaway.EndsAt.
func (s *setup) kyivTime(day, hour, minute int) *time.Time {
	t := time.Date(2026, 10, day, hour, minute, 0, 0, s.now.Location()).UTC()

	return &t
}

// menu is the Reply.Next that shows the field menu of giveaway id.
func menu(id uint) string { return "edit:menu:" + strconv.FormatUint(uint64(id), 10) }

// open publishes g for the admin and opens its field menu, as a forward through the edit
// button does.
func (s *setup) open(t *testing.T, g *giveaway.Giveaway) giveaway.Giveaway {
	t.Helper()

	p := publish(t, s.store, g)
	s.forwardEdit(t, p.ID)

	return p
}

func (s *setup) forwardEdit(t *testing.T, id uint) {
	t.Helper()

	ctx := t.Context()
	if err := s.ed.BeginEdit(ctx, adminID); err != nil {
		t.Fatal(err)
	}

	if r := s.ed.ForwardEdit(ctx, s.tg, adminID, channelID, 42); r.Next != menu(id) || r.Notice != "" {
		t.Fatalf("ForwardEdit = %+v, want the menu of %d", r, id)
	}
}

// change taps the field button of giveaway id and sends value.
func (s *setup) change(t *testing.T, id uint, field, value string) giveaway.Reply {
	t.Helper()

	r := s.ed.Field(t.Context(), s.tg, adminID, id, field)
	if r.Next == "" || r.Notice != "" {
		t.Fatalf("Field(%s) = %+v, want its prompt", field, r)
	}

	return s.ed.Value(t.Context(), s.tg, adminID, r.Next, value)
}

func (s *setup) read(t *testing.T, id uint) giveaway.Giveaway {
	t.Helper()

	g, err := s.store.ByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	return g
}

func TestEditForwardChecks(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, s *setup)
		want    i18n.Key
	}{
		{name: "no giveaway", prepare: func(*testing.T, *setup) {}, want: i18n.EditNotFound},
		{
			name: "another admin's draft",
			prepare: func(t *testing.T, s *setup) {
				t.Helper()

				g := &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID, Winners: 1}
				if err := s.store.SaveDraft(t.Context(), g); err != nil {
					t.Fatal(err)
				}
			},
			want: i18n.EditNotFound,
		},
		{
			name: "another admin's giveaway",
			prepare: func(t *testing.T, s *setup) {
				t.Helper()
				publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID})
			},
			want: i18n.EditNotYours,
		},
		{
			name: "finished",
			prepare: func(t *testing.T, s *setup) {
				t.Helper()

				g := publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
				s.store.finish(t, g.ID)
			},
			want: i18n.EditFinished,
		},
		{
			name: "not an admin any more",
			prepare: func(t *testing.T, s *setup) {
				t.Helper()
				publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
				s.tg.admin = false
			},
			want: i18n.EditNotAdmin,
		},
		{
			name: "bot cannot edit",
			prepare: func(t *testing.T, s *setup) {
				t.Helper()
				publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
				s.tg.noEdit = true
			},
			want: i18n.EditNoRights,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			ctx := t.Context()
			tt.prepare(t, s)

			if err := s.ed.BeginEdit(ctx, adminID); err != nil {
				t.Fatal(err)
			}

			if r := s.ed.ForwardEdit(ctx, s.tg, adminID, channelID, 42); r.Notice != tt.want || r.Next != "" {
				t.Errorf("ForwardEdit = %+v, want notice %q and the edit ended", r, tt.want)
			}

			if step := s.step(t); step != "" {
				t.Errorf("step = %q, want it cleared", step)
			}
		})
	}
}

func TestEditWinners(t *testing.T) {
	s := newSetup(t)
	g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"})

	if r := s.change(t, g.ID, giveaway.FieldWinners, "3"); r != (giveaway.Reply{Next: menu(g.ID)}) {
		t.Errorf("Value = %+v, want the menu again", r)
	}

	if got := s.read(t, g.ID); got.Winners != 3 || got.ButtonText != "Go" {
		t.Errorf("giveaway = %+v, want 3 winners and the same text", got)
	}

	if s.tg.attached != 0 || s.step(t) != "" {
		t.Errorf("attached %d, step %q; want no post edit and no step", s.tg.attached, s.step(t))
	}
}

func TestEditEndTime(t *testing.T) {
	s := newSetup(t)

	tests := []struct {
		name        string
		ends        *time.Time
		field, in   string
		want        *time.Time
		wantNotice  i18n.Key
		wantMessage int64
	}{
		{
			name: "date keeps the time", ends: s.kyivTime(20, 19, 0),
			field: giveaway.FieldEndDate, in: "2026-10-25", want: s.kyivTime(25, 19, 0),
		},
		{
			name: "time keeps the date", ends: s.kyivTime(20, 19, 0),
			field: giveaway.FieldEndTime, in: "18:30", want: s.kyivTime(20, 18, 30),
		},
		{
			name:  "date without an end time takes 19:00",
			field: giveaway.FieldEndDate, in: "2026-10-25", want: s.kyivTime(25, 19, 0),
		},
		{
			// A due giveaway gets a new future end and runs again.
			name: "date of a due giveaway", ends: s.kyivTime(8, 19, 0),
			field: giveaway.FieldEndDate, in: "2026-10-10", want: s.kyivTime(10, 19, 0),
		},
		{
			name: "today at a passed time", ends: s.kyivTime(20, 19, 0),
			field: giveaway.FieldEndDate, in: "2026-10-09", want: s.kyivTime(20, 19, 0),
			wantNotice: i18n.EditDateTimeInPast,
		},
		{
			// A due giveaway's old date is past, so a new time moves it to today.
			name: "time of a due giveaway", ends: s.kyivTime(8, 19, 0),
			field: giveaway.FieldEndTime, in: "22:00", want: s.kyivTime(9, 22, 0),
		},
		{
			name: "passed time", ends: s.kyivTime(9, 21, 0),
			field: giveaway.FieldEndTime, in: "19:00", want: s.kyivTime(9, 21, 0),
			wantNotice: i18n.GiveawayTimeInPast,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: tt.ends})

			r := s.change(t, g.ID, tt.field, tt.in)
			if r.Notice != tt.wantNotice {
				t.Errorf("Value = %+v, want notice %q", r, tt.wantNotice)
			}

			got := s.read(t, g.ID)
			if got.EndsAt == nil || !got.EndsAt.Equal(*tt.want) {
				t.Errorf("ends = %v, want %v", got.EndsAt, tt.want)
			}

			if tt.wantNotice != "" && s.step(t) != r.Next {
				t.Errorf("step = %q, want it kept at %q", s.step(t), r.Next)
			}

			if tt.wantNotice == "" && got.State(s.now) != giveaway.Active {
				t.Errorf("state = %v, want active", got.State(s.now))
			}
		})
	}
}

func TestEditButtonText(t *testing.T) {
	s := newSetup(t)
	g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"})

	s.tg.attachErr = errBoom
	if r := s.change(t, g.ID, giveaway.FieldButtonText, "Join"); r.Notice != i18n.EditNoRights {
		t.Errorf("Value with no edit right = %+v, want the no-rights notice", r)
	}

	if got := s.read(t, g.ID); got.ButtonText != "Go" {
		t.Errorf("text after a failed post edit = %q, want Go", got.ButtonText)
	}

	s.tg.attachErr = nil
	if r := s.change(t, g.ID, giveaway.FieldButtonText, "Join"); r != (giveaway.Reply{Next: menu(g.ID)}) {
		t.Errorf("Value = %+v, want the menu again", r)
	}

	if got := s.read(t, g.ID); got.ButtonText != "Join" || s.tg.label != "Join" {
		t.Errorf("text = %q, post label %q; want Join on both", got.ButtonText, s.tg.label)
	}
}

func TestEditButtonTextOfCancelled(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"})

	if err := s.store.Cancel(ctx, g.ID); err != nil {
		t.Fatal(err)
	}

	if r := s.change(t, g.ID, giveaway.FieldButtonText, "Join"); r != (giveaway.Reply{Next: menu(g.ID)}) {
		t.Errorf("Value = %+v, want the menu again", r)
	}

	if got := s.read(t, g.ID); got.ButtonText != "Join" || s.tg.attached != 0 {
		t.Errorf("text %q, attached %d; want Join saved and no button on the post", got.ButtonText, s.tg.attached)
	}
}

func TestEditValueWithoutStep(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	r := s.ed.Field(ctx, s.tg, adminID, g.ID, giveaway.FieldWinners)

	// A menu press clears the pending command.
	if err := s.users.SetPendingCommand(ctx, adminID, ""); err != nil {
		t.Fatal(err)
	}

	if got := s.ed.Value(ctx, s.tg, adminID, r.Next, "3"); !got.Stale {
		t.Errorf("Value after a menu press = %+v, want stale", got)
	}

	if s.read(t, g.ID).Winners == 3 {
		t.Error("winners saved after the wait stopped")
	}
}

// cancelOpen opens the menu of a published giveaway with a participant and cancels it.
func (s *setup) cancelOpen(t *testing.T) giveaway.Giveaway {
	t.Helper()

	ctx := t.Context()
	g := s.open(t, &giveaway.Giveaway{
		ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go", EndsAt: s.kyivTime(20, 19, 0),
	})

	if _, err := s.store.Join(ctx, g.ID, fanID); err != nil {
		t.Fatal(err)
	}

	wantAsk := "edit:cancel:" + strconv.FormatUint(uint64(g.ID), 10)
	if r := s.ed.AskCancel(ctx, s.tg, adminID, g.ID); r.Next != wantAsk {
		t.Errorf("AskCancel = %+v, want the confirmation", r)
	}

	want := giveaway.Reply{Notice: i18n.EditCancelled, Next: menu(g.ID)}
	if r := s.ed.Cancel(ctx, s.tg, s.tg, adminID, g.ID); r != want {
		t.Errorf("Cancel = %+v, want cancelled and the menu", r)
	}

	return g
}

func TestEditCancel(t *testing.T) {
	s := newSetup(t)
	g := s.cancelOpen(t)

	if got := s.read(t, g.ID); got.Published || s.tg.removed != 1 {
		t.Errorf("published %v, removed %d; want cancelled and the button removed", got.Published, s.tg.removed)
	}

	if r := s.ed.Cancel(t.Context(), s.tg, s.tg, adminID, g.ID); !r.Stale || s.tg.removed != 1 {
		t.Errorf("second Cancel = %+v, removed %d; want stale and no second removal", r, s.tg.removed)
	}
}

func TestEditFinishNow(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.open(t, &giveaway.Giveaway{
		ChatID: channelID, MessageID: 42, CreatedBy: adminID, Winners: 2, EndsAt: s.kyivTime(20, 19, 0),
	})

	wantAsk := "edit:finish:" + strconv.FormatUint(uint64(g.ID), 10)
	if r := s.ed.AskFinish(ctx, s.tg, adminID, g.ID); r.Next != wantAsk {
		t.Errorf("AskFinish = %+v, want the confirmation", r)
	}

	if r := s.ed.FinishNow(ctx, s.tg, adminID, g.ID); r != (giveaway.Reply{Notice: i18n.EditFinishing}) {
		t.Errorf("FinishNow = %+v, want finishing", r)
	}

	if got := s.read(t, g.ID); got.State(s.now) != giveaway.Due || got.Winners != 2 {
		t.Errorf("giveaway = %+v, want due now with its winner count kept", got)
	}

	if r := s.ed.FinishNow(ctx, s.tg, adminID, g.ID); !r.Stale {
		t.Errorf("second FinishNow = %+v, want stale", r)
	}
}

func TestEditRestore(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.cancelOpen(t)

	want := giveaway.Reply{Notice: i18n.EditRestored, Next: menu(g.ID)}
	if r := s.ed.Restore(ctx, s.tg, adminID, g.ID); r != want {
		t.Errorf("Restore = %+v, want restored and the menu", r)
	}

	if got := s.read(t, g.ID); !got.Published || s.tg.label != "Go" {
		t.Errorf("published %v, label %q; want published with the stored text", got.Published, s.tg.label)
	}

	if added, err := s.store.Join(ctx, g.ID, fanID); err != nil || added {
		t.Errorf("Join after restore = %v, %v; want the participant kept", added, err)
	}
}

func TestEditRestoreAfterEnd(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.open(
		t,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: s.kyivTime(8, 19, 0)},
	)

	if err := s.store.Cancel(ctx, g.ID); err != nil {
		t.Fatal(err)
	}

	if r := s.ed.Restore(ctx, s.tg, adminID, g.ID); r.Notice != i18n.EditEndPassed || r.Next != menu(g.ID) {
		t.Errorf("Restore = %+v, want the end-passed notice and the menu", r)
	}

	if got := s.read(t, g.ID); got.Published || s.tg.attached != 0 {
		t.Errorf("published %v, attached %d; want nothing published", got.Published, s.tg.attached)
	}
}

func TestEditDraft(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()

	d := &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Winners: 1}
	if err := s.store.SaveDraft(ctx, d); err != nil {
		t.Fatal(err)
	}

	draft, err := s.store.Draft(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}

	s.forwardEdit(t, draft.ID)

	if r := s.change(t, draft.ID, giveaway.FieldEndDate, "2026-10-25"); r != (giveaway.Reply{Next: menu(draft.ID)}) {
		t.Fatalf("change end date = %+v, want the menu", r)
	}

	want := giveaway.Reply{Notice: i18n.GiveawayCreated, Next: menu(draft.ID)}
	if r := s.ed.Restore(ctx, s.tg, adminID, draft.ID); r != want {
		t.Errorf("Restore of a draft = %+v, want created and the menu", r)
	}

	if got := s.read(t, draft.ID); !got.Published || got.EndsAt == nil || s.tg.label == "" {
		t.Errorf("published %v, ends %v, label %q; want published with the new end and a button",
			got.Published, got.EndsAt, s.tg.label)
	}
}

func TestEditStop(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()

	if err := s.ed.BeginEdit(ctx, adminID); err != nil {
		t.Fatal(err)
	}

	if r := s.ed.Stop(ctx, adminID); r != (giveaway.Reply{Notice: i18n.EditStopped}) || s.step(t) != "" {
		t.Errorf("Stop at the forward = %+v, step %q; want stopped and no step", r, s.step(t))
	}

	g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
	s.ed.Field(ctx, s.tg, adminID, g.ID, giveaway.FieldWinners)

	if r := s.ed.Stop(ctx, adminID); r != (giveaway.Reply{Next: menu(g.ID)}) || s.step(t) != "" {
		t.Errorf("Stop at a field = %+v, step %q; want the menu and no step", r, s.step(t))
	}

	if r := s.ed.Stop(ctx, adminID); !r.Stale {
		t.Errorf("Stop with no edit = %+v, want stale", r)
	}
}

func TestEditList(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()

	if got, err := s.ed.List(
		ctx,
		s.tg,
		adminID,
		"en",
		"test_bot",
	); err != nil ||
		got != "You have no active giveaways." {
		t.Errorf("empty List = %q, %v", got, err)
	}

	publish(
		t,
		s.store,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 41, CreatedBy: adminID, EndsAt: s.kyivTime(25, 19, 0)},
	)
	due := publish(
		t,
		s.store,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: s.kyivTime(8, 19, 0)},
	)

	for _, userID := range []int64{101, 102} {
		if _, err := s.store.Join(ctx, due.ID, userID); err != nil {
			t.Fatal(err)
		}
	}

	gone := publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 43, CreatedBy: adminID})
	if err := s.store.Cancel(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	draft := &giveaway.Giveaway{ChatID: channelID, MessageID: 44, CreatedBy: adminID, Winners: 1}
	if err := s.store.SaveDraft(ctx, draft); err != nil {
		t.Fatal(err)
	}

	got, err := s.ed.List(ctx, s.tg, adminID, "en", "test_bot")
	if err != nil {
		t.Fatal(err)
	}

	want := "Your active giveaways:\n\n" +
		`- 🟢 <a href="https://t.me/c/1234567890/42">Post</a> <a href="https://t.me/test_bot?start=edit_1234567890_42">[✏️]</a> · ` +
		`ends 2026-10-08 19:00, awaiting results · 👥 2` + "\n" +
		`- 🟢 <a href="https://t.me/c/1234567890/41">Post</a> <a href="https://t.me/test_bot?start=edit_1234567890_41">[✏️]</a> · ` +
		`ends 2026-10-25 19:00 · 👥 0` + "\n" +
		`- 📝 <a href="https://t.me/c/1234567890/44">Post</a> <a href="https://t.me/test_bot?start=edit_1234567890_44">[✏️]</a> · ` +
		`ends —, draft, not published · 👥 0`
	if got != want {
		t.Errorf("List = %q, want %q", got, want)
	}
}

func TestEditShowMenuOfAnotherCreator(t *testing.T) {
	s := newSetup(t)
	g := publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID})

	if r := s.ed.ShowMenu(t.Context(), s.tg, adminID, g.ID); r != (giveaway.Reply{Notice: i18n.EditNotYours}) {
		t.Errorf("ShowMenu of another creator's giveaway = %+v, want the not-yours notice", r)
	}

	if r := s.ed.ShowMenu(t.Context(), s.tg, fanID, g.ID); r.Next != menu(g.ID) {
		t.Errorf("ShowMenu by the creator = %+v, want the menu", r)
	}
}

func TestEditCancelWithoutRights(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	s.tg.removeErr = errBoom

	want := giveaway.Reply{Notice: i18n.EditNoRights, Next: menu(g.ID)}
	if r := s.ed.Cancel(ctx, s.tg, s.tg, adminID, g.ID); r != want {
		t.Errorf("Cancel = %+v, want no rights and the menu", r)
	}

	if !s.read(t, g.ID).Published {
		t.Error("giveaway cancelled although its button stayed on the post")
	}
}

func TestEditRestoreWithoutRights(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.cancelOpen(t)

	s.tg.attachErr = errBoom

	want := giveaway.Reply{Notice: i18n.EditNoRights, Next: menu(g.ID)}
	if r := s.ed.Restore(ctx, s.tg, adminID, g.ID); r != want {
		t.Errorf("Restore = %+v, want no rights and the menu", r)
	}

	if s.read(t, g.ID).Published {
		t.Error("giveaway published although its button is not on the post")
	}
}

func TestEditValueAfterAdminLoss(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()
	g := s.open(t, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Winners: 1})

	r := s.ed.Field(ctx, s.tg, adminID, g.ID, giveaway.FieldWinners)
	s.tg.admin = false

	if got := s.ed.Value(ctx, s.tg, adminID, r.Next, "3"); got != (giveaway.Reply{Notice: i18n.EditNotAdmin}) {
		t.Errorf("Value after losing admin = %+v, want the not-admin notice", got)
	}

	if s.read(t, g.ID).Winners != 1 || s.step(t) != "" {
		t.Errorf("winners %d, step %q; want 1 and no step", s.read(t, g.ID).Winners, s.step(t))
	}
}

func TestEditListPostGone(t *testing.T) {
	s := newSetup(t)
	publish(
		t,
		s.store,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: s.kyivTime(25, 19, 0)},
	)

	s.tg.attachErr = giveaway.ErrPostGone

	got, err := s.ed.List(t.Context(), s.tg, adminID, "en", "test_bot")
	if err != nil {
		t.Fatal(err)
	}

	want := "Your active giveaways:\n\n" +
		`- ⚠️ <a href="https://t.me/c/1234567890/42">Post</a> <a href="https://t.me/test_bot?start=edit_1234567890_42">[✏️]</a> · ` +
		`ends 2026-10-25 19:00, post deleted · 👥 0` + "\n\n" +
		"⚠️ A post was deleted. Its participants are kept: tap [✏️], then [✏️] next to “📄 Post”, " +
		"and send a new post of the same channel."
	if got != want {
		t.Errorf("List = %q, want %q", got, want)
	}
}

// Covers AE6.
func TestEditListPendingResults(t *testing.T) {
	line := `- 📣 <a href="https://t.me/c/1234567890/42">Post</a> ` +
		`<a href="https://t.me/test_bot?start=results_1234567890_42">[📣 Review results]</a> · results not published`

	tests := []struct {
		name   string
		userID int64
		admin  bool
		want   string
	}{
		{"creator", adminID, true, "Your active giveaways:\n\n" + line},
		{"another admin of the channel", fanID, true, "Your active giveaways:\n\n" + line},
		{"not an admin", fanID, false, "You have no active giveaways."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			g := publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
			s.store.draw(t, g.ID)
			s.tg.admin = tt.admin

			got, err := s.ed.List(t.Context(), s.tg, tt.userID, "en", "test_bot")
			if err != nil {
				t.Fatal(err)
			}

			if got != tt.want {
				t.Errorf("List = %q, want %q", got, tt.want)
			}

			// The running giveaways' post check attaches the button; a drawn post must stay without it.
			if s.tg.attached != 0 {
				t.Errorf("List attached the join button %d times, want none", s.tg.attached)
			}
		})
	}
}

func TestEditListSkipsPublishedResults(t *testing.T) {
	s := newSetup(t)
	g := publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
	s.store.finish(t, g.ID)
	s.tg.admin = true

	got, err := s.ed.List(t.Context(), s.tg, adminID, "en", "test_bot")
	if err != nil {
		t.Fatal(err)
	}

	if got != "You have no active giveaways." {
		t.Errorf("List = %q, want the empty list", got)
	}
}
