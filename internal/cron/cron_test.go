package cron_test

import (
	"context"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/cron"
)

func TestEveryStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	runs := 0
	cron.Every(ctx, time.Hour, func(context.Context) { runs++ })

	if runs != 0 {
		t.Errorf("ran %d times on a cancelled context, want 0", runs)
	}
}

func TestEveryRunsAtOnceThenEveryTick(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	runs := 0
	cron.Every(ctx, 20*time.Millisecond, func(context.Context) { runs++ })

	if runs < 2 {
		t.Errorf("ran %d times in 50ms with a 20ms interval, want a run at once and at least one tick", runs)
	}
}
