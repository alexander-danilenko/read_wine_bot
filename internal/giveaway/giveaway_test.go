package giveaway_test

import (
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

func TestState(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Hour), now.Add(-time.Hour)

	tests := []struct {
		name string
		g    giveaway.Giveaway
		want giveaway.State
	}{
		{name: "before the end", g: giveaway.Giveaway{EndsAt: &later}, want: giveaway.Active},
		{name: "after the end without winners", g: giveaway.Giveaway{EndsAt: &earlier}, want: giveaway.Due},
		{name: "at the end", g: giveaway.Giveaway{EndsAt: &now}, want: giveaway.Due},
		{
			name: "winners recorded",
			g:    giveaway.Giveaway{EndsAt: &earlier, FinishedAt: &earlier},
			want: giveaway.Finished,
		},
		{name: "no end time", g: giveaway.Giveaway{}, want: giveaway.Active},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.g.State(now); got != tt.want {
				t.Errorf("State = %v, want %v", got, tt.want)
			}
		})
	}
}
