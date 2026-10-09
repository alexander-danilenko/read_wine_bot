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
		want [4]string
	}{
		{lang: "uk", want: [4]string{"🎁 Новий розіграш", "✏️ Змінити розіграш", "📋 Мої розіграші", "🌐 Змінити мову"}},
		{lang: "en", want: [4]string{"🎁 New giveaway", "✏️ Edit giveaway", "📋 My giveaways", "🌐 Change language"}},
		{lang: "de", want: [4]string{"🎁 Новий розіграш", "✏️ Змінити розіграш", "📋 Мої розіграші", "🌐 Змінити мову"}},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			kb := menu.Keyboard(texts, tt.lang)
			if !kb.IsPersistent || !kb.ResizeKeyboard {
				t.Errorf("IsPersistent = %v, ResizeKeyboard = %v, want both true", kb.IsPersistent, kb.ResizeKeyboard)
			}

			if len(kb.Keyboard) != 2 || len(kb.Keyboard[0]) != 2 || len(kb.Keyboard[1]) != 2 {
				t.Fatalf("keyboard = %+v, want two rows of two buttons", kb.Keyboard)
			}

			got := [4]string{
				kb.Keyboard[0][0].Text,
				kb.Keyboard[0][1].Text,
				kb.Keyboard[1][0].Text,
				kb.Keyboard[1][1].Text,
			}
			if got != tt.want {
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
		"✏️ Edit giveaway": true,
		"📋 Мої розіграші":  true,
		"5":                false,
		"/start":           false,
	} {
		if got := isLabel(text); got != want {
			t.Errorf("IsLabel(%q) = %v, want %v", text, got, want)
		}
	}
}
