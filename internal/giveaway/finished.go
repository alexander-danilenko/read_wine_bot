package giveaway

import (
	"context"
	"strings"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// pageSize is how many giveaways one page of /list and /list_finished shows.
const pageSize = 25

// FinishedLister reads a creator's finished giveaways page by page. *Store implements it.
type FinishedLister interface {
	Finished(ctx context.Context, userID int64, offset, limit int) ([]Giveaway, error)
	WinnerCounts(ctx context.Context, ids []uint) (map[uint]int, error)
}

// Finished returns the HTML text of page, from 1, of the user's finished giveaways, the latest
// draw first, numbered across pages, and whether a next page exists. A giveaway whose results
// wait for review gets a link that opens the review in the bot named botUsername.
func (e *Editor) Finished(ctx context.Context, userID int64, page int, lang, botUsername string) (string, bool, error) {
	offset := (page - 1) * pageSize

	// One row more than a page tells whether a next page exists, without counting the rest.
	gs, err := e.finished.Finished(ctx, userID, offset, pageSize+1)
	if err != nil {
		return "", false, err //nolint:wrapcheck // Store already names the user
	}

	more := len(gs) > pageSize
	gs = gs[:min(len(gs), pageSize)]

	if len(gs) == 0 {
		return e.svc.texts.Text(lang, i18n.FinishedEmpty), false, nil
	}

	ids := make([]uint, 0, len(gs))
	for _, g := range gs {
		ids = append(ids, g.ID)
	}

	participants, err := e.counter.Participants(ctx, ids)
	if err != nil {
		return "", false, err //nolint:wrapcheck // Store already names the giveaways
	}

	winners, err := e.finished.WinnerCounts(ctx, ids)
	if err != nil {
		return "", false, err //nolint:wrapcheck // Store already names the giveaways
	}

	lines := make([]string, 0, len(gs))

	for i, g := range gs {
		line := e.svc.texts.Text(lang, i18n.FinishedItem, offset+i+1, postLink(g.ChatID, g.MessageID),
			g.FinishedAt.In(e.svc.kyiv).Format(dateLayout+" "+timeLayout), participants[g.ID], winners[g.ID])
		if g.ResultsPending {
			line += e.svc.texts.Text(lang, i18n.FinishedReview, resultsLink(botUsername, g.ChatID, g.MessageID))
		}

		lines = append(lines, line)
	}

	return e.svc.texts.Text(lang, i18n.FinishedList) + "\n\n" + strings.Join(lines, "\n"), more, nil
}
