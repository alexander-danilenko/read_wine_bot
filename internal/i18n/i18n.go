// Package i18n holds the bot's user-facing texts, one JSON file per language in locales/.
// schema.json lists the keys every locale file must have.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// DefaultLang is the language used when a user's language has no file in locales/.
const DefaultLang = "uk"

// Key names one user-facing text. Every key exists in every locales/*.json file.
type Key string

// Texts the bot sends. Placeholders use fmt's indexed verbs (%[1]s) so a translation can
// reorder them.
const (
	Greeting         Key = "greeting"
	CommandNew       Key = "command.new"
	CommandEdit      Key = "command.edit"
	CommandList      Key = "command.list"
	CommandLanguage  Key = "command.language"
	CommandCancel    Key = "command.cancel"
	CommandCancelled Key = "command.cancelled"
	CommandHelp      Key = "command.help"
	Help             Key = "help"
	LanguagePrompt   Key = "language.prompt"
	LanguageSelected Key = "language.selected"

	GiveawayPrompt            Key = "giveaway.prompt"
	GiveawayAskEndDate        Key = "giveaway.ask_end_date"
	GiveawayAskEndTime        Key = "giveaway.ask_end_time"
	GiveawayAskWinners        Key = "giveaway.ask_winners"
	GiveawayAskButtonText     Key = "giveaway.ask_button_text"
	GiveawayInvalidDate       Key = "giveaway.invalid_date"
	GiveawayDateInPast        Key = "giveaway.date_in_past"
	GiveawayInvalidTime       Key = "giveaway.invalid_time"
	GiveawayTimeInPast        Key = "giveaway.time_in_past"
	GiveawayInvalidWinners    Key = "giveaway.invalid_winners"
	GiveawayInvalidButtonText Key = "giveaway.invalid_button_text"
	GiveawaySummary           Key = "giveaway.summary"
	GiveawayConfirm           Key = "giveaway.confirm"
	GiveawayCancel            Key = "giveaway.cancel"
	GiveawayCancelled         Key = "giveaway.cancelled"
	GiveawayOutdated          Key = "giveaway.outdated"
	GiveawayCreated           Key = "giveaway.created"
	GiveawayExists            Key = "giveaway.exists"
	GiveawayTaken             Key = "giveaway.taken"
	GiveawayNotAdmin          Key = "giveaway.not_admin"
	GiveawayNoRights          Key = "giveaway.no_rights"
	GiveawayFailed            Key = "giveaway.failed"
	GiveawayButton            Key = "giveaway.button"
	GiveawayJoined            Key = "giveaway.joined"
	GiveawayAlreadyJoined     Key = "giveaway.already_joined"
	GiveawayJoinFailed        Key = "giveaway.join_failed"
	GiveawayEnded             Key = "giveaway.ended"
	DrawResults               Key = "draw.results"
	DrawResultsOne            Key = "draw.results_one"
	DrawCongrats              Key = "draw.congrats"
	DrawCongratsOne           Key = "draw.congrats_one"
	DrawNoWinners             Key = "draw.no_winners"

	EditPrompt            Key = "edit.prompt"
	EditNotFound          Key = "edit.not_found"
	EditNotYours          Key = "edit.not_yours"
	EditFinished          Key = "edit.finished"
	EditNotAdmin          Key = "edit.not_admin"
	EditNoRights          Key = "edit.no_rights"
	EditFailed            Key = "edit.failed"
	EditMenu              Key = "edit.menu"
	EditStatusPublished   Key = "edit.status_published"
	EditStatusCancelled   Key = "edit.status_cancelled"
	EditStatusDraft       Key = "edit.status_draft"
	EditLink              Key = "edit.link"
	EditCancelGiveaway    Key = "edit.cancel_giveaway"
	EditPublishGiveaway   Key = "edit.publish_giveaway"
	EditAskCancel         Key = "edit.ask_cancel"
	EditYes               Key = "edit.yes"
	EditNo                Key = "edit.no"
	EditCancelled         Key = "edit.cancelled"
	EditFinishGiveaway    Key = "edit.finish_giveaway"
	EditAskFinish         Key = "edit.ask_finish"
	EditYesFinish         Key = "edit.yes_finish"
	EditFinishing         Key = "edit.finishing"
	EditRestored          Key = "edit.restored"
	EditEndPassed         Key = "edit.end_passed"
	EditDateTimeInPast    Key = "edit.date_time_in_past"
	EditStopped           Key = "edit.stopped"
	EditList              Key = "edit.list"
	EditListItem          Key = "edit.list_item"
	EditListDue           Key = "edit.list_due"
	EditListDraft         Key = "edit.list_draft"
	EditListMarkPublished Key = "edit.list_mark_published"
	EditListMarkDraft     Key = "edit.list_mark_draft"
	EditListMarkGone      Key = "edit.list_mark_gone"
	EditListGone          Key = "edit.list_gone"
	EditListGoneNote      Key = "edit.list_gone_note"
	EditListEmpty         Key = "edit.list_empty"
	EditMovePrompt        Key = "edit.move_prompt"
	EditMoveOtherChannel  Key = "edit.move_other_channel"
	EditMoveSame          Key = "edit.move_same"
	EditMoveTaken         Key = "edit.move_taken"
	EditMoved             Key = "edit.moved"
	EditPostGone          Key = "edit.post_gone"

	ReviewResults         Key = "review.results"
	ReviewNoWinners       Key = "review.no_winners"
	ReviewRerollButton    Key = "review.reroll_button"
	ReviewPublishButton   Key = "review.publish_button"
	ReviewAskReroll       Key = "review.ask_reroll"
	ReviewYesReroll       Key = "review.yes_reroll"
	ReviewAskPublish      Key = "review.ask_publish"
	ReviewYesPublish      Key = "review.yes_publish"
	ReviewPublished       Key = "review.published"
	ReviewPublishFailed   Key = "review.publish_failed"
	ReviewPublishedPartly Key = "review.published_partly"
	ReviewSkipButton      Key = "review.skip_button"
	ReviewAskSkip         Key = "review.ask_skip"
	ReviewYesSkip         Key = "review.yes_skip"
	ReviewSkipped         Key = "review.skipped"
	ReviewNotAdmin        Key = "review.not_admin"
	ReviewNotPending      Key = "review.not_pending"
	ReviewWaiting         Key = "review.waiting"
	ReviewChannels        Key = "review.channels"
	EditListResults       Key = "edit.list_results"
	EditListMarkResults   Key = "edit.list_mark_results"
	CommandListFinished   Key = "command.list_finished"
	FinishedList          Key = "finished.list"
	FinishedItem          Key = "finished.item"
	FinishedReview        Key = "finished.review"
	FinishedEmpty         Key = "finished.empty"
	PagePrev              Key = "page.prev"
	PageNext              Key = "page.next"
)

// ErrMissingDefault means locales/ has no file for DefaultLang.
var ErrMissingDefault = errors.New("no locale file for " + DefaultLang)

// ErrKeysMismatch means a file's keys differ from the keys schema.json requires.
var ErrKeysMismatch = errors.New("translation keys differ from schema.json")

// schemaKey is the key editors read to find schema.json; it is not a translation.
const schemaKey = "$schema"

//go:embed schema.json locales/*.json
var files embed.FS

// schema is the part of schema.json that Load checks. It is a subset of JSON Schema: each
// locale file must have exactly the required keys, and every property must be required.
type schema struct {
	Properties map[Key]json.RawMessage `json:"properties"`
	Required   []Key                   `json:"required"`
}

// Catalog looks up texts by language and key.
type Catalog struct {
	texts map[string]map[Key]string // language -> key -> text
}

// Load parses schema.json and every embedded locale file, and checks that each locale file has
// exactly the keys schema.json requires. It reports every mismatched file in one error.
func Load() (*Catalog, error) {
	required, err := requiredKeys()
	if err != nil {
		return nil, err
	}

	entries, err := files.ReadDir("locales")
	if err != nil {
		return nil, fmt.Errorf("read locales: %w", err)
	}

	texts := make(map[string]map[Key]string, len(entries))
	var errs []error
	for _, e := range entries {
		data, err := files.ReadFile(path.Join("locales", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read locale %s: %w", e.Name(), err)
		}

		var m map[Key]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse locale %s: %w", e.Name(), err)
		}
		delete(m, schemaKey)

		if diff := keyDiff(required, slices.Collect(maps.Keys(m))); len(diff) > 0 {
			errs = append(errs, fmt.Errorf("locale %s: %w: %v", e.Name(), ErrKeysMismatch, diff))
		}

		texts[strings.TrimSuffix(e.Name(), ".json")] = m
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	if _, ok := texts[DefaultLang]; !ok {
		return nil, ErrMissingDefault
	}

	return &Catalog{texts: texts}, nil
}

// requiredKeys returns the keys schema.json requires, after checking that they match its
// properties, so a key added to only one of the two lists is caught.
func requiredKeys() ([]Key, error) {
	data, err := files.ReadFile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("read schema.json: %w", err)
	}

	var s schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse schema.json: %w", err)
	}
	delete(s.Properties, schemaKey)

	if diff := keyDiff(s.Required, slices.Collect(maps.Keys(s.Properties))); len(diff) > 0 {
		return nil, fmt.Errorf("schema.json required vs properties: %w: %v", ErrKeysMismatch, diff)
	}

	return s.Required, nil
}

// Text returns the text for key in lang, with args filled in the way fmt.Sprintf does.
// lang is a Telegram language_code such as "uk" or "en-US"; a language without a locale
// file falls back to DefaultLang.
func (c *Catalog) Text(lang string, key Key, args ...any) string {
	base, _, _ := strings.Cut(lang, "-")

	m, ok := c.texts[base]
	if !ok {
		m = c.texts[DefaultLang]
	}

	text, ok := m[key]
	if !ok {
		return string(key)
	}

	return fmt.Sprintf(text, args...)
}

// Langs returns the language of every locale file, sorted.
func (c *Catalog) Langs() []string {
	return slices.Sorted(maps.Keys(c.texts))
}

// keyDiff returns the keys present in only one of want and got, sorted.
func keyDiff(want, got []Key) []Key {
	var diff []Key
	for _, k := range want {
		if !slices.Contains(got, k) {
			diff = append(diff, k)
		}
	}

	for _, k := range got {
		if !slices.Contains(want, k) {
			diff = append(diff, k)
		}
	}

	slices.Sort(diff)

	return diff
}
