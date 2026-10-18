---
title: Describe every column, table and index in the database
date: 2026-10-18
category: conventions
module: internal/db, internal/giveaway, internal/user
problem_type: convention
component: database
severity: low
applies_when:
  - Adding or changing a GORM model struct, column, table or named index
  - Wondering why startup runs COMMENT ON statements
root_cause: missing_documentation
tags: [gorm, postgres, automigrate, comment-tag, schema-docs, comment-on]
---

# Describe every column, table and index in the database

## Context

Database tools (DBeaver, DataGrip, pgAdmin, `psql \d+`) show a PostgreSQL object's description next to it. The owner wants every column, table and index to carry one, so the data can be read without the Go source. This replaced the SQLite-era rule "document columns with Go comments, not GORM `comment:` tags": SQLite had nowhere to store a description, PostgreSQL does.

## Guidance

- **Columns:** every field that maps to a column gets a `comment:` in its `gorm` tag. `gorm.io/driver/postgres` writes it with `COMMENT ON COLUMN` when it creates the table or adds the column, and its `MigrateColumn` rewrites the description on every start when the tag text changed (`gorm.io/driver/postgres@v1.6.3/migrator.go:308`).
- **Tables and named indexes:** GORM has no tag for them, so `descriptions()` in `internal/db/db.go` lists `COMMENT ON TABLE` / `COMMENT ON INDEX` statements that `db.Open` runs after `AutoMigrate`. `COMMENT ON` accepts no bind parameters, so each text is a SQL literal; double any `'`.
- **Tag text:** no `;` (it ends the tag setting) and no `"` (it ends the Go struct tag). Write for someone reading the data: what the value means, its unit, and what NULL or empty means.
- Go comments stay for what only matters to the code (GORM quirks, why a default exists).

`TestEverythingDescribed` (`internal/db/db_internal_test.go`) parses the models with `gorm.io/gorm/schema` and fails when a column has no `comment:` tag or a table or named index has no entry in `descriptions()`. It needs no database.

## Example

```go
Winners int `gorm:"not null;default:1;comment:Number of winners to draw"`
```

```go
`COMMENT ON TABLE giveaway_participants IS 'Users who tapped a giveaway''s join button, with the winners marked after the draw'`,
```
