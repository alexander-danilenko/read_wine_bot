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
const DefaultLang = "en"

// Key names one user-facing text. Every key exists in every locales/*.json file.
type Key string

// Texts the bot sends. Placeholders use fmt's indexed verbs (%[1]s) so a translation can
// reorder them.
const (
	Greeting         Key = "greeting"
	LanguagePrompt   Key = "language.prompt"
	LanguageSelected Key = "language.selected"
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
