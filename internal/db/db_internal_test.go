package db

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestEverythingDescribed fails when a column, table or named index has no description, so
// database tools always show what each one holds. It parses the models; no database runs.
func TestEverythingDescribed(t *testing.T) {
	stmts := descriptions()
	described := func(kind, name string) bool {
		return slices.ContainsFunc(stmts, func(s string) bool {
			return strings.HasPrefix(s, "COMMENT ON "+kind+" "+name+" IS ")
		})
	}

	for _, m := range models() {
		s, err := schema.Parse(m, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatal(err)
		}

		if !described("TABLE", s.Table) {
			t.Errorf("table %s has no COMMENT ON TABLE in descriptions", s.Table)
		}

		for _, f := range s.Fields {
			if f.DBName != "" && f.Comment == "" {
				t.Errorf("column %s.%s has no comment: tag", s.Table, f.DBName)
			}
		}

		for _, idx := range s.ParseIndexes() {
			if !described("INDEX", idx.Name) {
				t.Errorf("index %s has no COMMENT ON INDEX in descriptions", idx.Name)
			}
		}
	}
}
