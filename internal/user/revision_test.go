package user_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// revisions returns the user's revisions, oldest first.
func revisions(t *testing.T, db *gorm.DB, id int64) []user.Revision {
	t.Helper()

	var revs []user.Revision
	if err := db.Where("user_id = ?", id).Order("number").Find(&revs).Error; err != nil {
		t.Fatal(err)
	}

	return revs
}

// profileOf returns the profile a revision holds.
func profileOf(r user.Revision) user.Profile {
	return user.Profile{
		TelegramID:   r.UserID,
		IsBot:        r.IsBot,
		FirstName:    r.FirstName,
		LastName:     r.LastName,
		Username:     r.Username,
		LanguageCode: r.LanguageCode,
		IsPremium:    r.IsPremium,
	}
}

// record saves each profile in turn.
func record(t *testing.T, store *user.Store, profiles ...user.Profile) {
	t.Helper()

	for _, p := range profiles {
		if err := store.Record(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecordNewUser(t *testing.T) {
	const id int64 = 7_000_000_010

	store, db := newStore(t)
	p := user.Profile{
		TelegramID: id, FirstName: "Olena", LastName: "Koval", Username: "olena", LanguageCode: "uk", IsPremium: true,
	}
	record(t, store, p)

	want := user.User{
		TelegramID: id, FirstName: "Olena", LastName: "Koval", Username: "olena", LanguageCode: "uk",
		IsPremium: true, Revision: 1,
	}
	got := readUser(t, db, id)
	got.CreatedAt, got.UpdatedAt = want.CreatedAt, want.UpdatedAt

	if got != want {
		t.Errorf("new user row = %+v, want %+v", got, want)
	}

	revs := revisions(t, db, id)
	if len(revs) != 1 || revs[0].Number != 1 || profileOf(revs[0]) != p {
		t.Errorf("revisions = %+v; want one revision 1 holding %+v", revs, p)
	}
}

// TestRecordRevisions covers which profile changes write a revision and what each one holds.
func TestRecordRevisions(t *testing.T) {
	const id int64 = 7_000_000_011

	anna := user.Profile{TelegramID: id, FirstName: "Anna", LastName: "Koval", LanguageCode: "uk"}
	anya := anna
	anya.FirstName = "Anya"

	noLastName := anna
	noLastName.LastName = ""

	withUsername := anna
	withUsername.Username = "anna"

	premium := anna
	premium.IsPremium = true

	noLanguage := anna
	noLanguage.LanguageCode = ""

	renamedNoLanguage := anya
	renamedNoLanguage.LanguageCode = ""

	tests := []struct {
		name  string
		steps []user.Profile
		want  []user.Profile // every revision, oldest first
	}{
		{"same profile twice", []user.Profile{anna, anna}, []user.Profile{anna}},
		{"rename", []user.Profile{anna, anya}, []user.Profile{anna, anya}},
		{"rename and back", []user.Profile{anna, anya, anna}, []user.Profile{anna, anya, anna}},
		{"last name removed", []user.Profile{anna, noLastName}, []user.Profile{anna, noLastName}},
		{"username added", []user.Profile{anna, withUsername}, []user.Profile{anna, withUsername}},
		{"premium flipped", []user.Profile{anna, premium}, []user.Profile{anna, premium}},
		{"empty language code keeps the saved one", []user.Profile{anna, noLanguage}, []user.Profile{anna}},
		{"rename without language code keeps it", []user.Profile{anna, renamedNoLanguage}, []user.Profile{anna, anya}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, db := newStore(t)
			record(t, store, tt.steps...)

			revs := revisions(t, db, id)
			if len(revs) != len(tt.want) {
				t.Fatalf("got %d revisions %+v, want %d", len(revs), revs, len(tt.want))
			}

			for i, r := range revs {
				if r.Number != i+1 || profileOf(r) != tt.want[i] {
					t.Errorf("revision %d = number %d, %+v; want number %d, %+v",
						i, r.Number, profileOf(r), i+1, tt.want[i])
				}
			}

			if u := readUser(
				t,
				db,
				id,
			); u.Revision != len(tt.want) ||
				u.FirstName != tt.want[len(tt.want)-1].FirstName ||
				u.LanguageCode != tt.want[len(tt.want)-1].LanguageCode {
				t.Errorf("user row = %+v; want the latest revision %d", u, len(tt.want))
			}
		})
	}
}

// TestRecordKeepsBotState checks that the menu language and the pending command neither
// create revisions nor get overwritten by Record.
func TestRecordKeepsBotState(t *testing.T) {
	const id int64 = 7_000_000_012

	store, db := newStore(t)
	ctx := t.Context()
	p := user.Profile{TelegramID: id, FirstName: "Olena"}

	record(t, store, p)

	if err := store.SetLanguage(ctx, id, "uk"); err != nil {
		t.Fatal(err)
	}

	if err := store.SetPendingCommand(ctx, id, "giveaway:post"); err != nil {
		t.Fatal(err)
	}

	record(t, store, p)

	if revs := revisions(t, db, id); len(revs) != 1 {
		t.Errorf("got %d revisions, want 1", len(revs))
	}

	if u := readUser(t, db, id); u.Language != "uk" || u.PendingCommand != "giveaway:post" {
		t.Errorf("after Record: language %q, pending %q; want uk, giveaway:post", u.Language, u.PendingCommand)
	}
}

// TestRecordAfterSetLanguage covers a user row that SetLanguage created before any Record.
func TestRecordAfterSetLanguage(t *testing.T) {
	const id int64 = 7_000_000_013

	store, db := newStore(t)

	if err := store.SetLanguage(t.Context(), id, "en"); err != nil {
		t.Fatal(err)
	}

	p := user.Profile{TelegramID: id, FirstName: "Olena"}
	record(t, store, p)

	revs := revisions(t, db, id)
	if len(revs) != 1 || revs[0].Number != 1 || profileOf(revs[0]) != p {
		t.Errorf("revisions = %+v; want one revision 1 holding %+v", revs, p)
	}

	if u := readUser(t, db, id); u.Revision != 1 || u.Language != "en" {
		t.Errorf("user row = %+v; want revision 1, language en", u)
	}
}

func TestRevisionNumberUnique(t *testing.T) {
	const id int64 = 7_000_000_014

	store, db := newStore(t)
	record(t, store, user.Profile{TelegramID: id, FirstName: "Olena"})

	if err := db.Create(&user.Revision{UserID: id, Number: 1, FirstName: "Copy"}).Error; err == nil {
		t.Error("second revision 1 for the same user was saved; want a unique index error")
	}
}

// createUsers saves users the way the bot did before revisions existed.
func createUsers(t *testing.T, db *gorm.DB, users ...user.User) {
	t.Helper()

	for _, u := range users {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
}

// backfill runs BackfillRevisions twice, as a restart would.
func backfill(t *testing.T, db *gorm.DB) {
	t.Helper()

	for range 2 {
		if err := user.BackfillRevisions(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBackfillRevisions(t *testing.T) {
	_, db := newStore(t)
	long := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	createUsers(t, db,
		user.User{TelegramID: 1, FirstName: "Olena", LastName: "Koval", CreatedAt: long},
		user.User{TelegramID: 2, FirstName: "Taras", CreatedAt: long},
	)

	start := time.Now()
	backfill(t, db)

	for id, name := range map[int64]string{1: "Olena", 2: "Taras"} {
		revs := revisions(t, db, id)
		if len(revs) != 1 || revs[0].Number != 1 || revs[0].FirstName != name || revs[0].CreatedAt.Before(start) {
			t.Errorf("user %d revisions = %+v; want one revision 1 named %s, recorded at the backfill", id, revs, name)
		}

		if u := readUser(t, db, id); u.Revision != 1 {
			t.Errorf("user %d revision = %d, want 1", id, u.Revision)
		}
	}
}

func TestBackfillSkipsUsersWithRevisions(t *testing.T) {
	_, db := newStore(t)
	createUsers(t, db, user.User{TelegramID: 3, FirstName: "Ivan", Revision: 3})
	backfill(t, db)

	if revs := revisions(t, db, 3); len(revs) != 0 {
		t.Errorf("user at revision 3 got backfilled revisions %+v", revs)
	}
}

func TestRecordAfterBackfill(t *testing.T) {
	store, db := newStore(t)
	createUsers(t, db, user.User{TelegramID: 1, FirstName: "Olena", LastName: "Koval"})
	backfill(t, db)

	record(t, store, user.Profile{TelegramID: 1, FirstName: "Olena", LastName: "Koval", Username: "olena"})

	if revs := revisions(t, db, 1); len(revs) != 2 || revs[1].Number != 2 || revs[1].Username != "olena" {
		t.Errorf("after a new username: revisions = %+v; want revision 2 with username olena", revs)
	}
}
