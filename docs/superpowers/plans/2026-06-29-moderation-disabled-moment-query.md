# Moderation Disabled Moment Query Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure `moderation.enabled: false` makes every public moment query use the legacy `moment.status = 1` predicate without touching moderation tables.

**Architecture:** Inject the moderation visibility switch once when constructing the moment repository. The repository owns the SQL choice: disabled uses the legacy predicate; enabled keeps the placeholder-aware join. Service and repository method signatures stay unchanged.

**Tech Stack:** Go 1.25+, GORM/MySQL, go-sqlmock, testify.

## Global Constraints

- `moderation.enabled: false` must not query `moderation_item`.
- `moderation.enabled: true` must preserve medium-risk placeholder visibility.
- No global infrastructure state; dependencies remain constructor-injected.

---

### Task 1: Inject the public-query mode into the moment repository

**Files:**
- Modify: `internal/repository/moment/moment.go`
- Modify: `internal/repository/moment/query.go`
- Modify: `internal/repository/moment/moment_test.go`
- Modify: `internal/router/router.go`

**Interfaces:**
- Consumes: `config.Moderation.Enabled` from the router composition root.
- Produces: `NewMomentRepository(db *gorm.DB, moderationEnabled bool) MomentRepository`.

- [x] **Step 1: Write the failing repository test**

Add a sqlmock test that constructs the repository with `false`, expects `SELECT count(*) FROM moment WHERE moment.status = ? AND moment.user_id = ?`, and returns a public count without any `moderation_item` expectation. Keep the existing placeholder test in enabled mode.

- [x] **Step 2: Run test to verify RED**

Run: `go test ./internal/repository/moment -run 'TestMomentRepository_CountPublicByUser_BypassesModerationWhenDisabled' -count=1`

Expected: build failure because `NewMomentRepository` does not yet accept the switch.

- [x] **Step 3: Write the minimal implementation**

Store `moderationEnabled bool` on `momentRepo`. In `publicMomentBase`, return:

```go
query := r.db.Model(&model.Moment{})
if !r.moderationEnabled {
	return query.Where("moment.status = ?", uint8(1))
}
```

Otherwise retain the current moderation join and predicate. Pass `cfg.Moderation.Enabled` from `internal/router/router.go`, and update repository tests to explicitly choose enabled or disabled mode.

- [x] **Step 4: Run focused and full verification**

Run:

```bash
go test ./internal/repository/moment ./internal/router -count=1
go test ./... -count=1
```

Expected: both commands exit successfully with no failing packages.
