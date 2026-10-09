// Package menu builds the keyboard of buttons under the message field in the bot's private
// chat. The buttons replace slash commands: pressing one sends its label as a message.
package menu

import (
	"slices"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// Keyboard returns the menu keyboard with labels in lang. It stays visible until replaced.
func Keyboard(texts *i18n.Catalog, lang string) *models.ReplyKeyboardMarkup {
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{{
			{Text: texts.Text(lang, i18n.MenuGiveaway)},
			{Text: texts.Text(lang, i18n.MenuLanguage)},
		}},
		IsPersistent:   true,
		ResizeKeyboard: true,
	}
}

// IsLabel returns a function that reports whether a message text is a menu button label in
// any language, so a keyboard sent before a language switch still counts.
func IsLabel(texts *i18n.Catalog) func(text string) bool {
	labels := slices.Concat(texts.All(i18n.MenuGiveaway), texts.All(i18n.MenuLanguage))

	return func(text string) bool { return slices.Contains(labels, text) }
}
