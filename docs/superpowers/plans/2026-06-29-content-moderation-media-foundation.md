# Content Moderation Media Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the durable image snapshot and global approval records required before moderation can accept images or run `legacy_migration`.

**Architecture:** Keep immutable per-revision image facts separate from the reusable global approval projection. `moderation_revision_image` preserves ordered version snapshots; `moderation_image` identifies globally approved bytes by `sha256 + size` while retaining MD5 only as a lookup hint.

**Tech Stack:** Go 1.25+, GORM/MySQL, testify, versioned SQL migrations.

## Global Constraints

- SHA-256 plus byte size is the final trusted identity; MD5 alone never proves approval.
- Existing object keys and business IDs are never rewritten by this schema task.
- Image status is closed to `pending` and `approved`.
- `last_used_at` is persisted for configurable retention cleanup.

---

### Task 1: Add media persistence contracts

**Files:**
- Modify: `internal/model/moderation_content.go`
- Modify: `internal/model/moderation_contract_test.go`
- Modify: `internal/dbschema/schema.go`
- Modify: `internal/dbschema/seed_test.go`
- Create: `migrations/20260629_content_moderation_media.sql`

**Interfaces:**
- Produces: `model.ModerationRevisionImage`, `model.ModerationImage`, and `model.ModerationImageStatuses()`.
- Consumed later by: moderation image repository, preview processor, review transitions, cleanup, and `legacy_migration`.

- [x] **Step 1: Write failing model and schema registration tests**

Assert both table names, the `(revision_id, seq)` and `(sha256, size)` composite unique indexes, the closed status values, the exact SHA-256/MD5 field sizes, and dependency-ordered AutoMigrate registration.

- [x] **Step 2: Run tests and verify RED**

Run: `go test ./internal/model ./internal/dbschema -run 'Moderation' -count=1`

Expected: build failure because the new models and enum function do not exist.

- [x] **Step 3: Implement the model contracts and SQL migration**

Add immutable revision image fields `revision_id`, `seq`, `object_key`, `sha256`, `md5`, `size`, `media_type`, `is_gif`, and timestamps. Add global image fields `sha256`, `size`, `md5`, `status`, nullable `preview_object_key`, `approved_at`, `approved_by`, `last_used_at`, and timestamps. Register both after `ModerationRevision` and create the matching idempotent SQL migration with foreign keys and checks.

- [x] **Step 4: Verify focused and full suites**

Run:

```bash
go test ./internal/model ./internal/dbschema -count=1
go test ./... -count=1
```

Expected: both commands exit successfully with no failing packages.
