# Guestbook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build dedicated guestbook APIs for listing, creating, liking, and deleting messages.

**Architecture:** Add a focused guestbook module across DTO, repository, service, handler, and router. Reuse the existing `guestbook` table for messages and `user_like` for likes with type `5`.

**Tech Stack:** Go, Gin, GORM/MySQL, sqlmock, testify, swaggo.

---

### Task 1: DTO And Service Contract

**Files:**
- Create: `internal/dto/guestbook.go`
- Create: `internal/service/guestbook/guestbook.go`
- Create: `internal/service/guestbook/service.go`
- Test: `internal/service/guestbook/guestbook_test.go`

- [ ] Write failing service tests for default `owner_user_id=1`, content trimming, blank content rejection, and delete force for admin.
- [ ] Run `go test ./internal/service/guestbook -run TestGuestbookService -count=1` and verify the package does not compile yet because the module is missing.
- [ ] Implement DTOs, service interface, service errors, mapper, default owner normalization, content cleaning, and role permission forwarding.
- [ ] Re-run `go test ./internal/service/guestbook -run TestGuestbookService -count=1` and verify it passes.

### Task 2: Repository

**Files:**
- Create: `internal/repository/guestbook/guestbook.go`
- Create: `internal/repository/guestbook/query.go`
- Create: `internal/repository/guestbook/mutation.go`
- Create: `internal/repository/guestbook/user.go`
- Test: `internal/repository/guestbook/guestbook_test.go`
- Modify: `internal/model/user.go`

- [ ] Write failing repository tests for list default shape, like toggle creation, and owner/author delete permission.
- [ ] Run `go test ./internal/repository/guestbook -run TestGuestbookRepository -count=1` and verify the tests fail before implementation.
- [ ] Implement owner existence checks, list pagination, user loading, like counts, toggle like transaction, and soft delete permission checks.
- [ ] Re-run `go test ./internal/repository/guestbook -run TestGuestbookRepository -count=1` and verify it passes.

### Task 3: Handler And Router

**Files:**
- Create: `internal/handler/guestbook/guestbook.go`
- Create: `internal/handler/guestbook/query.go`
- Create: `internal/handler/guestbook/mutation.go`
- Create: `internal/handler/guestbook/response.go`
- Test: `internal/handler/guestbook/guestbook_test.go`
- Modify: `internal/router/router.go`

- [ ] Write failing handler tests for list default owner, create using claims user ID, like path binding, and delete role forwarding.
- [ ] Run `go test ./internal/handler/guestbook -run TestGuestbookHandler -count=1` and verify the tests fail before implementation.
- [ ] Implement handler methods with Swagger comments and wire `GET /guestbook`, `POST /guestbook`, `POST /guestbook/:id/like`, `DELETE /guestbook/:id`.
- [ ] Re-run `go test ./internal/handler/guestbook -run TestGuestbookHandler -count=1` and verify it passes.

### Task 4: Documentation And Regression

**Files:**
- Modify: `docs/docs.go`
- Modify: `docs/swagger.json`
- Modify: `docs/swagger.yaml`

- [ ] Run focused guestbook tests.
- [ ] Run `go test ./internal/service/guestbook ./internal/repository/guestbook ./internal/handler/guestbook -count=1`.
- [ ] Run `make swag` and confirm `/guestbook` paths appear in generated Swagger files.
- [ ] Run broader relevant tests or `go test ./...` if dependency cache permits.
