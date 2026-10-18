---
title: Document columns with Go comments, not GORM comment tags
date: 2026-10-09
category: conventions
module: internal/giveaway, internal/user
problem_type: convention
component: database
severity: low
applies_when:
  - Adding or changing a GORM model struct that AutoMigrate turns into a SQLite table
  - Wanting a description of a column or an index to live next to the schema
root_cause: wrong_api
retire_when: "github.com/glebarez/sqlite emits COMMENT for columns or indexes; check its migrator.go CreateIndex and a probe AutoMigrate on a newer tag"
tags: [gorm, sqlite, glebarez, automigrate, comment-tag, schema-docs]
---

# Document columns with Go comments, not GORM comment tags

## Context

The bot stores giveaways and users in SQLite through GORM `AutoMigrate`. While designing the giveaway tables, the question came up how to attach a description to each column. GORM has a `comment:` tag (`gorm:"comment:..."`) and an index option (`index:name,comment:...`), so the natural move is to put the descriptions there.

SQLite itself has no column or index comments, and `github.com/glebarez/sqlite` v1.11.0 (the driver this repo must use, see CLAUDE.md) silently drops both. An earlier session concluded that an index `comment:` breaks `AutoMigrate`, citing `gorm.io/gorm` `migrator/migrator.go:855`, which appends `COMMENT '...'` to `CREATE INDEX`. That conclusion was wrong for this driver: glebarez overrides `CreateIndex` (`github.com/glebarez/sqlite@v1.11.0/migrator.go:252`) and its version never reads `idx.Comment`, so GORM's generic line 855 is never reached.

A probe on 2026-10-09 (gorm v1.31.2, glebarez/sqlite v1.11.0) declared `Name string \`gorm:"comment:the name;index:idx_name,comment:idx comment"\`` and ran `AutoMigrate` twice. Both runs succeeded and SQLite stored:

```sql
CREATE TABLE `things` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text)
CREATE INDEX `idx_name` ON `things`(`name`)
```

No error, no comment, no column re-alter on the second run.

## Guidance

Describe a column with a Go line comment on the struct field, and leave `comment:` out of `gorm` tags:

```go
ChatID    int64 `gorm:"not null;uniqueIndex:idx_giveaways_post"` // the channel
MessageID int64 `gorm:"not null;uniqueIndex:idx_giveaways_post"` // the post, numbered per channel
```

This is the pattern in `internal/giveaway/giveaway.go:18-23`.

## Why This Matters

A `comment:` tag looks like schema documentation but reaches nothing: not the database file, not `sqlite3 .schema`, not any tool reading the DB. A reader of the struct trusts it as documented, and the next engineer either repeats the investigation or repeats the wrong "it breaks AutoMigrate" claim. Go comments are read by everyone who changes the schema, because the struct is the schema.

## When to Apply

Every GORM model in this repo. Since the move to PostgreSQL (2026-10-18), GORM's Postgres migrator does write `comment:` tags as `COMMENT ON COLUMN`, so the SQLite finding above no longer holds; Go comments stay the convention because the struct is the schema and is what every editor reads. Switching to `comment:` tags is an open choice, not a rule.

## Examples

Before (looks documented, stores nothing):

```go
Winners int `gorm:"not null;default:1;comment:number of winners to draw"`
```

After:

```go
Winners int `gorm:"not null;default:1"` // number of winners to draw
```
