package giveaway_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// finishAt publishes a giveaway of creator on post and marks it finished at the moment.
func (s *setup) finishAt(t *testing.T, creator, post int64, at time.Time) giveaway.Giveaway {
	t.Helper()

	g := publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: post, CreatedBy: creator})

	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	s.store.rows[g.ID].FinishedAt = &at

	return *s.store.rows[g.ID]
}

func TestFinishedLine(t *testing.T) {
	s := newSetup(t)
	g := s.finishAt(t, adminID, 42, *s.kyivTime(30, 19, 0))

	for _, userID := range []int64{adminID, fanID} {
		if _, err := s.store.Join(t.Context(), g.ID, userID); err != nil {
			t.Fatal(err)
		}
	}

	s.store.winners[g.ID] = 1
	s.store.rows[g.ID].ResultsPending = true

	got, more, err := s.ed.Finished(t.Context(), adminID, 1, "en", "test_bot")
	if err != nil {
		t.Fatal(err)
	}

	want := "Your finished giveaways:\n\n" +
		`1. <a href="https://t.me/c/1234567890/42">Post</a> · 2026-10-30 19:00 · 👥 2 · 📣 1` +
		` · <a href="https://t.me/test_bot?start=results_1234567890_42">[📣 Review]</a>`
	if got != want || more {
		t.Errorf("Finished = %q, more %v; want %q, no more", got, more, want)
	}
}

func TestFinishedPages(t *testing.T) {
	s := newSetup(t)
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	n := int64(2*giveaway.PageSize + 1) // two full pages and one line on the third

	// Post i finished i minutes after start, so post n is the latest.
	for i := int64(1); i <= n; i++ {
		s.finishAt(t, adminID, i, start.Add(time.Duration(i)*time.Minute))
	}

	// Neither another admin's finished giveaway nor a running one is listed.
	s.finishAt(t, fanID, 1000, start)
	publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 1001, CreatedBy: adminID})

	for page := 1; page <= 3; page++ {
		t.Run("page "+strconv.Itoa(page), func(t *testing.T) {
			got, more, err := s.ed.Finished(t.Context(), adminID, page, "en", "test_bot")
			if err != nil {
				t.Fatal(err)
			}

			first := (page-1)*giveaway.PageSize + 1
			want := min(giveaway.PageSize, int(n)-first+1)

			lines := strings.Split(got, "\n")[2:]
			if len(lines) != want || more != (page < 3) {
				t.Fatalf("page %d: %d lines, more %v; want %d lines, more %v", page, len(lines), more, want, page < 3)
			}

			for i, line := range lines {
				number := first + i
				prefix := strconv.Itoa(number) + `. <a href="https://t.me/c/1234567890/` +
					strconv.FormatInt(n-int64(number)+1, 10) + `">`
				if !strings.HasPrefix(line, prefix) {
					t.Errorf("line %d = %q, want it to start with %q", i, line, prefix)
				}
			}
		})
	}
}

func TestFinishedEmpty(t *testing.T) {
	s := newSetup(t)
	publish(t, s.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	got, more, err := s.ed.Finished(t.Context(), adminID, 1, "en", "test_bot")
	if err != nil {
		t.Fatal(err)
	}

	if got != "You have no finished giveaways." || more {
		t.Errorf("Finished = %q, more %v; want the empty text, no more", got, more)
	}
}
