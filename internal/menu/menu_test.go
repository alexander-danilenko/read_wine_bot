package menu_test

import (
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/menu"
)

func TestKeyboard(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		lang string
		want [2]string
	}{
		{lang: "uk", want: [2]string{"🎁 Новий розіграш", "🌐 Змінити мову"}},
		{lang: "de", want: [2]string{"🎁 New giveaway", "🌐 Change language"}},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			kb := menu.Keyboard(texts, tt.lang)
			if !kb.IsPersistent || !kb.ResizeKeyboard {
				t.Errorf("IsPersistent = %v, ResizeKeyboard = %v, want both true", kb.IsPersistent, kb.ResizeKeyboard)
			}

			if len(kb.Keyboard) != 1 || len(kb.Keyboard[0]) != 2 {
				t.Fatalf("keyboard = %+v, want one row of two buttons", kb.Keyboard)
			}

			if got := [2]string{kb.Keyboard[0][0].Text, kb.Keyboard[0][1].Text}; got != tt.want {
				t.Errorf("labels = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsLabel(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	isLabel := menu.IsLabel(texts)

	for text, want := range map[string]bool{
		"🎁 New giveaway":   true,
		"🎁 Новий розіграш": true,
		"🌐 Змінити мову":   true,
		"5":                false,
		"/start":           false,
	} {
		if got := isLabel(text); got != want {
			t.Errorf("IsLabel(%q) = %v, want %v", text, got, want)
		}
	}
}
