# Schema challenge protocol

Mandatory before writing a GORM model, a `Store` query or anything else that changes the shape of data.

1. State the access patterns it serves: the queries, the write paths and the expected row counts.
2. Check these rules. A rejected change is not implemented as asked.
   - A `jsonb` or array column whose data is filtered, joined, grows without limit or has its own lifecycle -> propose a table.
   - A relationship without a foreign key ("we validate in the app") -> reject. In GORM, declare it with an association field, as `Giveaway.Creator` does.
   - A uniqueness rule or invariant enforced only in Go -> propose a unique index or a `CHECK` constraint.
   - A nullable column whose `comment:` does not say what NULL means -> flag.
   - A polymorphic association (`entity_type` + `entity_id`) -> reject, propose separate foreign keys or a supertype table.
   - An EAV table (key/value rows for attributes) -> reject, unless users really define the attributes.
   - Money in `float`/`real` -> reject, use `numeric` or an integer of minor units.
   - Event time as `timestamp` without time zone -> reject, use `timestamptz` (GORM's default for `time.Time`).
   - A copied (denormalized) column without a named sync mechanism (same transaction, trigger) -> reject.
   - A query that loads rows and filters them in Go when a `WHERE` or an index can do it -> reject, propose the index or a new shape.
   - A row updated more than about once per second, sustained -> reject (row-lock contention), propose insert-only rows aggregated on read.
   - One write that fans out to many rows with no cap on how many -> flag.
   - A change that makes `AutoMigrate` rewrite or lock a large table (type change, new index on a big table: `AutoMigrate` cannot `CREATE INDEX CONCURRENTLY`) -> require an expand/contract plan.
   - A new `CHECK` or unique constraint -> first query production for rows that break it and fix them, or `AutoMigrate` cannot add the constraint and the bot fails at startup. Mirror the constraint in the store's in-memory fake, or tests pass on data the database rejects.
3. On a violation, reply with the rule, the concrete failure in production and the alternative. Implement only after the owner explicitly picks one option.
4. Every accepted rule break gets an ADR in `docs/adr/` (`NNNN-<slug>.md`: context, decision, consequences).
