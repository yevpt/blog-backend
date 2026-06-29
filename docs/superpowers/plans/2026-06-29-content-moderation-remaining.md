# Content Moderation Remaining Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the remaining image, governance, operations, migration, and production-readiness work for the personal-blog moderation system.

**Architecture:** Keep moderation orchestration in `internal/service/moderation`, durable facts in `internal/repository/moderation`, object processing in a focused `internal/service/moderationmedia` package, and operational scheduling in `internal/worker/moderation`. Database transitions stay atomic; Garage operations use prepare-before-transaction and best-effort compensation after failure.

**Tech Stack:** Go 1.25+, Gin, GORM/MySQL, Garage S3, zap, viper, go-sqlmock, gomock, httptest/testify.

## Global Constraints

- Text alone determines low, medium, or high risk; images never change textual risk.
- Unapproved static images expose only a low-resolution preview; unapproved GIF images expose only the configured GIF placeholder.
- SHA-256 plus byte size is the trusted global image identity; an MD5 filename only narrows candidates.
- Editing never destroys the last approved image set before a new version is approved.
- Deleted moderation items are terminal; emergency restore only restores `state_before_emergency` and never deleted content.
- Reporter and appeal APIs remain out of scope.
- `moderation.enabled: false` must keep legacy reads and writes independent of moderation tables.
- All thresholds, retention periods, batch sizes, paths, and public notices remain in strong typed config.

---

### Task 1: Image inspection, fingerprinting, and preview preparation

**Files:**
- Create: `internal/service/moderationmedia/media.go`
- Create: `internal/service/moderationmedia/inspect.go`
- Create: `internal/service/moderationmedia/preview.go`
- Create: `internal/service/moderationmedia/media_test.go`
- Modify: `pkg/storage/resolver.go`
- Modify: `pkg/storage/storage.go`

**Interfaces:**
- Produces `moderationmedia.Service.Prepare(ctx, userID, []string) (PreparedSet, error)`.
- `PreparedImage` contains `ObjectKey`, `SHA256`, `MD5`, `Size`, `MediaType`, `IsGIF`, `PreviewObjectKey`, and `Approved`.
- Consumes a constructor-injected object reader/store and an image repository lookup boundary.

- [x] Write failing tests for exact byte fingerprints, decoded-pixel and byte limits, approved-image reuse updating access time, 48-pixel static previews, GIF placeholders, duplicate order preservation, ownership-safe object keys, and cleanup of newly-created previews on repository failure.
- [x] Run `go test ./internal/service/moderationmedia -count=1` and confirm RED because the package does not exist.
- [x] Add `storage.ObjectReader` with `GetObject`, then implement bounded reads, `image.DecodeConfig` validation, SHA-256/MD5 calculation, preview generation through `pkg/imageutil`, and deterministic preview keys below `moderation/previews/`.
- [x] Run the focused tests and `go test ./pkg/storage ./internal/service/moderationmedia -count=1`.
- [x] Commit with `feat(moderation): 实现审核图片指纹与预览处理`.

### Task 2: Persist revision images and global approval reuse

**Files:**
- Modify: `internal/repository/moderation/repository.go`
- Modify: `internal/repository/moderation/types.go`
- Create: `internal/repository/moderation/image.go`
- Create: `internal/repository/moderation/image_test.go`
- Modify: `internal/repository/moderation/transition.go`
- Modify: `internal/repository/moderation/repository_test.go`
- Regenerate: `internal/repository/moderation/mock/mock_repository.go`

**Interfaces:**
- Produces atomic `UseApprovedImage`, `UpsertPendingImage`, and `LoadRevisionImages` repository methods; obsolete-image selection remains in Task 6.
- Extends `RevisionDraft` with ordered `Images []RevisionImageDraft`.
- Review transitions approve every image fingerprint referenced by the approved revision in the same MySQL transaction.

- [x] Write sqlmock tests proving MD5 lookup also requires SHA-256 and size, approved hits update `last_used_at`, revision order is immutable, transition rollback removes no prior snapshot, and review approval records `approved_at/approved_by`.
- [x] Run repository tests and confirm RED on missing interfaces.
- [x] Implement the named repository methods and transition inserts/updates without exposing GORM models to services.
- [x] Regenerate repository mocks with the repository's existing `go generate` command and run `go test ./internal/repository/moderation ./internal/service/moderation -count=1`.
- [x] Commit with `feat(moderation): 持久化审核版本图片与全站复用状态`.

### Task 3: Route moment, comment, reply, and guestbook images through moderation

**Files:**
- Modify: `internal/service/moderation/service.go`
- Modify: `internal/service/moderation/service_write.go`
- Modify: `internal/service/moderation/review.go`
- Modify: `internal/service/moderation/review_mapping.go`
- Modify: `internal/service/moderation/service_test.go`
- Modify: `internal/service/moderation/review_test.go`
- Modify: `internal/service/moment/moderation_write.go`
- Modify: `internal/service/moment/image.go`
- Modify: `internal/service/moment/mapper.go`
- Modify: `internal/service/commentasset/assets.go`
- Modify: `internal/service/comment/moderation_write.go`
- Modify: `internal/service/guestbook/moderation_write.go`
- Modify: `internal/service/moment/moment_test.go`
- Modify: `internal/service/comment/comment_test.go`
- Modify: `internal/service/guestbook/guestbook_test.go`
- Modify: `internal/router/moderation.go`
- Modify: `internal/router/router.go`

**Interfaces:**
- `moderation.Service` receives real ordered object keys instead of sentinel image markers.
- `SubmitResult` and `View` expose safe ordered display image keys only; business DTO mappers resolve those keys to signed URLs.
- Medium edits keep the old approved content and images; low edits expose the new content with preview/placeholder images.

- [x] Write service tests for first publish and edit across approved/unapproved/static/GIF combinations, preserving old images for medium edits and restoring them on rejection.
- [x] Run moment/comment/guestbook/moderation tests and confirm RED on `ErrImageReviewUnavailable` or missing image projections.
- [x] Normalize comment temp images into durable owner-scoped keys, prepare moment files before submission with compensation, call the media service, and persist real ordered snapshots.
- [x] Remove the unconditional image rejection, feed `HasUnapprovedImage` into policy selection, return safe image projections, and delete a preview only after its image becomes approved.
- [x] Run `go test ./internal/service/moderation ./internal/service/moment ./internal/service/comment ./internal/service/guestbook -count=1`.
- [x] Commit with `feat(moderation): 接入全站内容图片审核流程`.

### Task 4: Complete user trust governance and sanctions

**Files:**
- Modify: `internal/repository/moderation/repository.go`
- Create: `internal/repository/moderation/governance.go`
- Create: `internal/repository/moderation/governance_test.go`
- Create: `internal/service/moderation/governance.go`
- Create: `internal/service/moderation/governance_test.go`
- Modify: `internal/service/moderation/service_write.go`
- Modify: `internal/service/auth/auth.go`
- Modify: `internal/service/auth/auth_test.go`
- Modify: `internal/router/router.go`

**Interfaces:**
- Produces `GovernanceService.EnsureNewProfile`, `GetProfile`, `SetTrust`, `SetSanction`, and `ReleaseSanction`.
- Missing profiles remain fail-safe `new + active`; legacy migration creates `trusted + active` profiles.
- Automatic promotion and restriction use existing `moderation.governance` thresholds; manual trust locks block automatic changes.

- [x] Write gomock/sqlmock tests for new-user creation, clean approval promotion, corrected/rejected/high-risk scoring, restricted expiry, manual locks, mute/ban publish rejection, and idempotent counters.
- [x] Run focused tests and confirm RED on missing governance methods.
- [x] Implement profile repository/service operations, update high-risk attempts and review transitions atomically, and best-effort profile creation after registration.
- [x] Run `go test ./internal/repository/moderation ./internal/service/moderation ./internal/service/auth -count=1`.
- [x] Commit with `feat(moderation): 完成用户信任等级与处罚治理`.

### Task 5: Add admin governance, global control, and emergency operations APIs

**Files:**
- Create: `internal/dto/admin_moderation_operations.go`
- Create: `internal/handler/moderation/operations.go`
- Create: `internal/handler/moderation/operations_test.go`
- Create: `internal/service/moderation/operations.go`
- Create: `internal/service/moderation/operations_test.go`
- Create: `internal/repository/moderation/operations.go`
- Modify: `internal/router/moderation_admin.go`
- Modify: `internal/router/router.go`
- Regenerate: `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`.

**API Risk Pass:**
- Caller: admin only; author identity never comes from the request body.
- Bounds: reasons use `review.reason_max_chars`; batches use `control.user_hide_batch_size` and `user_hide_max_items_per_request`.
- Cost: all mass actions are cursor-based, bounded, synchronous, and rate limited.
- Failure: each batch is one MySQL transaction; no half-updated item inside a committed batch.
- Cleanup: terminal deletion remains irreversible; restore clears only the emergency snapshot fields.

- [ ] Write handler/service/repository tests for profile inspection/manual correction, mute/ban/release, registration/publishing switches, single-item hide/restore, cursor-based user hide, and rejected deleted-content restore.
- [ ] Run focused tests and confirm RED on missing routes.
- [ ] Implement DTO-only responses, admin handlers, bounded operations, cached control reads, and explicit routes that are absent when moderation is disabled.
- [ ] Run `make swag`, verify no `model.*` appears in Swagger, and run handler/router tests.
- [ ] Commit with `feat(moderation): 新增审核治理与紧急处置接口`.

### Task 6: Add bounded cleanup workers

**Files:**
- Create: `internal/worker/moderation/cleanup.go`
- Create: `internal/worker/moderation/cleanup_test.go`
- Create: `internal/repository/moderation/cleanup.go`
- Create: `internal/repository/moderation/cleanup_test.go`
- Modify: `internal/router/router.go`

**Interfaces:**
- Worker runs only when moderation is enabled.
- Uses `moderation.image` and `moderation.audit` retention/batch config.
- Never removes current materialized, approved, pending, or retained revision references.

- [ ] Write tests for expired attempts/action logs/obsolete revisions, stale image records, orphan previews, temp objects, active-reference protection, minimum object age, bounded batches, and Garage failure logging without process exit.
- [ ] Run worker/repository tests and confirm RED.
- [ ] Implement repository selection/deletion and a cancellable ticker worker with injected logger, clock, repository, and object store.
- [ ] Run focused tests plus `go test ./internal/worker/... -count=1`.
- [ ] Commit with `feat(moderation): 新增审核记录与图片定期清理`.

### Task 7: Implement resumable legacy migration and verification

**Files:**
- Create: `internal/repository/moderation/migration.go`
- Create: `internal/repository/moderation/migration_test.go`
- Create: `internal/service/moderationmigration/migration.go`
- Create: `internal/service/moderationmigration/migration_test.go`
- Create: `cmd/moderation-migrate/main.go`
- Create: `cmd/moderation-migrate/main_test.go`
- Modify: `pkg/config/moderation.go`
- Modify: all config YAML files with migration batch defaults.

**Interfaces:**
- CLI supports `--batch-size`, `--after-type`, `--after-id`, `--verify-only`, and prints the next cursor.
- Backfills seven business content types as approved revision version 1 with decision `legacy_migration`.
- Backfills current content, moment options, ordered images, global approved fingerprints, `trusted + active` user profiles, and singleton control.

- [ ] Write sqlmock/service tests for all seven types, hidden moments, soft-deleted rows, ordered moment and embedded comment images, missing objects, idempotent reruns, cursor resume, and verification count/fingerprint failures.
- [ ] Run migration package tests and confirm RED.
- [ ] Implement per-batch transactions with `INSERT ... ON DUPLICATE KEY` semantics, bounded object reads, explicit cursor output, and a verification-only pass that performs no writes.
- [ ] Run the CLI tests and `go test ./cmd/moderation-migrate ./internal/service/moderationmigration ./internal/repository/moderation -count=1`.
- [ ] Commit with `feat(moderation): 新增历史内容审核迁移与校验`.

### Task 8: Production readiness gate

**Files:**
- Modify: `docs/superpowers/specs/2026-06-27-content-moderation-design.md`
- Create: `docs/moderation-rollout.md`
- Modify: config examples only if verification exposes missing defaults.

**Interfaces:**
- Documents the exact order: deploy with `enabled: false`, apply schema, run migration, deploy frontend idempotency keys, run verify-only, enable `enforce`, then smoke test.

- [ ] Add lifecycle tests covering create/edit low-medium-high, image preview/GIF placeholder, approve/correct/reject, delete terminal state, emergency hide/restore, governance, disabled fallback, and migrated content.
- [ ] Run `make swag`, `go test ./... -count=1`, `go vet ./...`, `git diff --check`, and repository invariant searches.
- [ ] Confirm production config remains `enabled: false`; do not enable it in code.
- [ ] Commit with `docs(moderation): 补充审核生产启用与回滚手册`.
