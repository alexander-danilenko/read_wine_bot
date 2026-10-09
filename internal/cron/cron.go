// Package cron runs the bot's regular tasks. main starts every one of them, so the list of
// what the bot does on a schedule lives in one place there.
package cron

import (
	"context"
	"time"
)

// Every runs task at once and then every interval, until ctx is cancelled. Runs never
// overlap: a run that takes longer than interval delays the next one.
func Every(ctx context.Context, interval time.Duration, task func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for ctx.Err() == nil {
		task(ctx)

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}
