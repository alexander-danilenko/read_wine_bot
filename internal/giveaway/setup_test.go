package giveaway_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// setup is a Service on real stores with a fake Telegram and a clock the test moves.
type setup struct {
	svc   *giveaway.Service
	store *giveaway.Store
	users *user.Store
	tg    *fakeTelegram
	now   time.Time
}

func newSetup(t *testing.T) *setup {
	t.Helper()

	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}

	store, conn := newStore(t)
	s := &setup{store: store, users: user.NewStore(conn), tg: &fakeTelegram{admin: true}}
	s.now = time.Date(2026, 10, 9, 19, 55, 0, 0, kyiv)
	s.svc = giveaway.NewService(
		store,
		store,
		s.users,
		texts,
		kyiv,
		func() time.Time { return s.now },
		slog.New(slog.DiscardHandler),
	)

	return s
}

func (s *setup) step(t *testing.T) string {
	t.Helper()

	step, err := s.users.PendingCommand(t.Context(), adminID)
	if err != nil {
		t.Fatal(err)
	}

	return step
}

// reachSummary walks the admin through every step with valid values.
func (s *setup) reachSummary(t *testing.T) {
	t.Helper()

	ctx := t.Context()
	if err := s.svc.Begin(ctx, adminID); err != nil {
		t.Fatal(err)
	}

	if r := s.svc.Forward(ctx, s.tg, adminID, channelID, 42, "uk"); r.Next != giveaway.StepEndDate {
		t.Fatalf("Forward = %+v, want next end date", r)
	}

	for _, in := range []struct{ step, value, next string }{
		{giveaway.StepEndDate, "2026-10-25", giveaway.StepEndTime},
		{giveaway.StepEndTime, "19:00", giveaway.StepWinners},
		{giveaway.StepWinners, "3", giveaway.StepButtonText},
		{giveaway.StepButtonText, "Беру участь", giveaway.StepConfirm},
	} {
		if r := s.svc.Apply(ctx, adminID, in.step, in.value); r.Next != in.next || r.Notice != "" {
			t.Fatalf("Apply(%s, %q) = %+v, want next %s", in.step, in.value, r, in.next)
		}
	}
}

func TestSetupSummary(t *testing.T) {
	s := newSetup(t)
	s.reachSummary(t)

	summary, err := s.svc.Summary(t.Context(), adminID, "en")
	if err != nil {
		t.Fatal(err)
	}

	want := "Check the giveaway:\n\nPost: https://t.me/c/1234567890/42\nEnds: 2026-10-25 19:00 (Kyiv time)\nWinners: 3\nButton: Беру участь"
	if summary != want {
		t.Errorf("Summary = %q, want %q", summary, want)
	}
}

func TestSetupConfirmCreates(t *testing.T) {
	s := newSetup(t)
	s.reachSummary(t)
	ctx := t.Context()

	if r := s.svc.Confirm(ctx, s.tg, adminID); r != (giveaway.Reply{Notice: i18n.GiveawayCreated}) {
		t.Fatalf("Confirm = %+v, want created", r)
	}

	g, err := s.store.Find(ctx, channelID, 42)
	if err != nil {
		t.Fatal(err)
	}

	ends := time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)
	if g.EndsAt == nil || !g.EndsAt.Equal(ends) || g.Winners != 3 || g.ButtonText != "Беру участь" ||
		g.Language != "uk" || g.CreatedBy != adminID {
		t.Errorf("giveaway = %+v; want ends %v, 3 winners, the chosen button text, uk, by the admin", g, ends)
	}

	if s.tg.label != "Беру участь" {
		t.Errorf("attached label %q, want the chosen text", s.tg.label)
	}
}

func TestSetupConfirmTwice(t *testing.T) {
	s := newSetup(t)
	s.reachSummary(t)
	ctx := t.Context()

	s.svc.Confirm(ctx, s.tg, adminID)

	if _, err := s.store.Draft(ctx, adminID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("draft after Confirm: err = %v, want ErrNotFound", err)
	}

	if r := s.svc.Confirm(ctx, s.tg, adminID); !r.Stale || s.tg.attached != 1 {
		t.Errorf("second Confirm = %+v, attached %d; want stale and one attach in total", r, s.tg.attached)
	}
}

func TestSetupForwardChecks(t *testing.T) {
	tests := []struct {
		name     string
		existing bool
		tg       *fakeTelegram
		want     i18n.Key
	}{
		{name: "giveaway exists", existing: true, tg: &fakeTelegram{admin: true}, want: i18n.GiveawayExists},
		{name: "not admin", tg: &fakeTelegram{}, want: i18n.GiveawayNotAdmin},
		{name: "bot cannot edit", tg: &fakeTelegram{admin: true, noEdit: true}, want: i18n.GiveawayNoRights},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			ctx := t.Context()

			if tt.existing {
				publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID})
			}

			if err := s.svc.Begin(ctx, adminID); err != nil {
				t.Fatal(err)
			}

			if r := s.svc.Forward(
				ctx,
				tt.tg,
				adminID,
				channelID,
				42,
				"uk",
			); r.Notice != tt.want ||
				r.Next != "" {
				t.Errorf("Forward = %+v, want notice %q and the setup ended", r, tt.want)
			}

			if step := s.step(t); step != "" {
				t.Errorf("step = %q, want none", step)
			}

			if _, err := s.store.Draft(ctx, adminID); !errors.Is(err, giveaway.ErrNotFound) {
				t.Errorf("draft: err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestSetupForwardTwice(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()

	if err := s.svc.Begin(ctx, adminID); err != nil {
		t.Fatal(err)
	}

	s.svc.Forward(ctx, s.tg, adminID, channelID, 42, "uk")

	if r := s.svc.Forward(ctx, s.tg, adminID, channelID, 43, "uk"); !r.Stale {
		t.Errorf("second Forward = %+v, want stale", r)
	}

	if s.tg.adminChecks != 1 {
		t.Errorf("admin checked %d times, want once", s.tg.adminChecks)
	}

	if d, err := s.store.Draft(ctx, adminID); err != nil || d.MessageID != 42 {
		t.Errorf("draft = %+v, %v; want the first post", d, err)
	}
}

func TestSetupApplyInvalidKeepsStep(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()

	if err := s.svc.Begin(ctx, adminID); err != nil {
		t.Fatal(err)
	}

	s.svc.Forward(ctx, s.tg, adminID, channelID, 42, "uk")

	r := s.svc.Apply(ctx, adminID, giveaway.StepEndDate, "25.10.2026")
	if r.Notice != i18n.GiveawayInvalidDate || r.Next != giveaway.StepEndDate {
		t.Errorf("Apply invalid date = %+v, want the date error and the date step again", r)
	}

	// A default button left over from a later step changes nothing.
	if r := s.svc.Apply(ctx, adminID, giveaway.StepEndTime, "19:00"); !r.Stale {
		t.Errorf("Apply at a step the admin is not at = %+v, want stale", r)
	}

	if d, err := s.store.Draft(ctx, adminID); err != nil || d.EndsAt != nil {
		t.Errorf("draft = %+v, %v; want no end time", d, err)
	}
}

func TestSetupConfirmRechecks(t *testing.T) {
	tests := []struct {
		name     string
		change   func(t *testing.T, s *setup)
		want     i18n.Key
		wantStep string
	}{
		{
			// Another admin forwards the same post, takes the draft over and publishes it.
			name: "giveaway created meanwhile",
			change: func(t *testing.T, s *setup) {
				t.Helper()

				publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID})
			},
			want: i18n.GiveawayExists,
		},
		{
			name:     "bot lost the edit right",
			change:   func(_ *testing.T, s *setup) { s.tg.attachErr = errBoom },
			want:     i18n.GiveawayNoRights,
			wantStep: giveaway.StepConfirm,
		},
		{
			name:     "end time passed today",
			change:   func(_ *testing.T, s *setup) { s.now = time.Date(2026, 10, 25, 19, 30, 0, 0, s.now.Location()) },
			want:     i18n.GiveawayTimeInPast,
			wantStep: giveaway.StepEndTime,
		},
		{
			name:     "end date passed",
			change:   func(_ *testing.T, s *setup) { s.now = time.Date(2026, 10, 26, 9, 0, 0, 0, s.now.Location()) },
			want:     i18n.GiveawayDateInPast,
			wantStep: giveaway.StepEndDate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			s.reachSummary(t)
			tt.change(t, s)

			if r := s.svc.Confirm(t.Context(), s.tg, adminID); r.Notice != tt.want || r.Next != tt.wantStep {
				t.Errorf("Confirm = %+v, want notice %q and next %q", r, tt.want, tt.wantStep)
			}

			if s.tg.attached != 0 {
				t.Errorf("attached %d time(s), want none", s.tg.attached)
			}

			if step := s.step(t); step != tt.wantStep {
				t.Errorf("step = %q, want %q", step, tt.wantStep)
			}
		})
	}
}

func TestSetupCancel(t *testing.T) {
	s := newSetup(t)
	ctx := t.Context()

	if r := s.svc.Cancel(ctx, adminID); !r.Stale {
		t.Errorf("Cancel without a setup = %+v, want stale", r)
	}

	s.reachSummary(t)

	if r := s.svc.Cancel(ctx, adminID); r.Notice != i18n.GiveawayCancelled || r.Next != "" {
		t.Errorf("Cancel = %+v, want cancelled", r)
	}

	if step := s.step(t); step != "" {
		t.Errorf("step = %q, want none", step)
	}

	if _, err := s.store.Draft(ctx, adminID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("draft after Cancel: err = %v, want ErrNotFound", err)
	}
}

func TestDefault(t *testing.T) {
	s := newSetup(t)

	for _, tt := range []struct{ step, lang, want string }{
		{giveaway.StepEndTime, "uk", "19:00"},
		{giveaway.StepWinners, "en", "1"},
		{giveaway.StepButtonText, "uk", "Приймаю участь"},
		{giveaway.StepEndDate, "uk", ""},
	} {
		if got := s.svc.Default(tt.step, tt.lang); got != tt.want {
			t.Errorf("Default(%s, %s) = %q, want %q", tt.step, tt.lang, got, tt.want)
		}
	}
}

// A crafted default button for a step that takes no value changes nothing.
func TestSetupApplyStepWithoutValue(t *testing.T) {
	s := newSetup(t)
	s.reachSummary(t)

	if r := s.svc.Apply(t.Context(), adminID, giveaway.StepConfirm, "x"); !r.Stale {
		t.Errorf("Apply at the confirm step = %+v, want stale", r)
	}

	if d, err := s.store.Draft(t.Context(), adminID); err != nil || d.ButtonText != "Беру участь" {
		t.Errorf("draft = %+v, %v; want the button text unchanged", d, err)
	}
}

// A failed attach can be a passing Telegram error, so Confirm keeps the draft and offers
// the summary again instead of making the admin start over.
func TestSetupConfirmKeepsDraftOnFailure(t *testing.T) {
	s := newSetup(t)
	s.reachSummary(t)
	ctx := t.Context()
	s.tg.attachErr = errBoom

	want := giveaway.Reply{Notice: i18n.GiveawayNoRights, Next: giveaway.StepConfirm}
	if r := s.svc.Confirm(ctx, s.tg, adminID); r != want {
		t.Errorf("Confirm = %+v, want %+v", r, want)
	}

	if step := s.step(t); step != giveaway.StepConfirm {
		t.Errorf("step = %q, want confirm", step)
	}

	if _, err := s.store.Draft(ctx, adminID); err != nil {
		t.Errorf("draft after failed Confirm: %v, want it kept", err)
	}

	s.tg.attachErr = nil
	if r := s.svc.Confirm(ctx, s.tg, adminID); r.Notice != i18n.GiveawayCreated {
		t.Errorf("retried Confirm = %+v, want created", r)
	}
}
