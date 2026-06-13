# Friendlink Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build public and admin CRUD APIs for friend links with DTO isolation, REST routes, Swagger docs, tests, and CDN resolution for `avatar_url`.

**Architecture:** Follow the existing category/tag pattern: handler binds and responds, service validates and maps DTOs, repository owns GORM access and returns `model.FriendLink`. `avatar_url` remains stored as a raw external URL or object key and is resolved in service with `storage.ObjectURLResolver`.

**Tech Stack:** Go 1.25+, Gin, GORM, go-sqlmock, testify, swaggo/swag, existing `pkg/response` and `pkg/storage`.

---

## File Structure

- Create `internal/dto/friendlink.go`: request and response DTOs only.
- Modify `internal/repository/friendlink.go`: repository interface and GORM implementation.
- Modify `internal/service/friendlink.go`: business interface, validation, pagination, DTO mapping, CDN URL resolution.
- Modify `internal/handler/friendlink.go`: HTTP handlers, request binding, error mapping, Swagger annotations.
- Modify `internal/router/router.go`: dependency wiring and route registration.
- Create `internal/service/friendlink_test.go`: service behavior tests with fake repository and resolver.
- Create `internal/handler/friendlink_test.go`: handler binding/error mapping tests with fake service.
- Create `internal/repository/friendlink_test.go`: repository SQL behavior tests with go-sqlmock.
- Regenerate `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml` with `make swag`.

## Task 1: DTO And Service Contract

**Files:**
- Create: `internal/dto/friendlink.go`
- Modify: `internal/service/friendlink.go`
- Test: `internal/service/friendlink_test.go`

- [ ] **Step 1: Write failing service tests**

Add tests for:

```go
func TestFriendLinkService_ListPublic_ResolvesAvatarURL(t *testing.T)
func TestFriendLinkService_GetPublic_HiddenLinkReturnsNotFound(t *testing.T)
func TestFriendLinkService_Create_DefaultsStatusAndTrimsFields(t *testing.T)
func TestFriendLinkService_Update_AllowsClearingOptionalFields(t *testing.T)
func TestFriendLinkService_Update_RejectsInvalidStatus(t *testing.T)
```

- [ ] **Step 2: Run service tests and verify RED**

Run: `go test ./internal/service -run 'TestFriendLinkService' -count=1`

Expected: FAIL because friendlink DTOs, service methods, and repository contract do not exist yet.

- [ ] **Step 3: Implement DTO and service**

Implement:

```go
type FriendLinkListReq struct { Page int; PageSize int; Status *uint8 }
type FriendLinkCreateReq struct { Name string; Description *string; Email *string; Phone *string; Site string; AvatarUrl *string; Seq *uint; Status *uint8 }
type FriendLinkUpdateReq struct { Name *string; Description *string; Email *string; Phone *string; Site *string; AvatarUrl *string; Seq *uint; Status *uint8 }
type FriendLinkItemResp struct { ID uint; Name string; Description *string; Email *string; Phone *string; Site string; AvatarUrl *string; Seq uint; Status uint8; CreatedAt time.Time; UpdatedAt time.Time }
type FriendLinkPageResp struct { Total int64; Pages int; Page int; PageSize int; List []FriendLinkItemResp }
```

Add service interface:

```go
type FriendLinkService interface {
    ListPublic(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error)
    GetPublic(id uint) (*dto.FriendLinkItemResp, error)
    ListAdmin(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error)
    Create(req dto.FriendLinkCreateReq) (*dto.FriendLinkItemResp, error)
    Update(id uint, req dto.FriendLinkUpdateReq) (*dto.FriendLinkItemResp, error)
    Delete(id uint) (*dto.FriendLinkItemResp, error)
}
```

- [ ] **Step 4: Run service tests and verify GREEN**

Run: `go test ./internal/service -run 'TestFriendLinkService' -count=1`

Expected: PASS.

## Task 2: Repository Implementation

**Files:**
- Modify: `internal/repository/friendlink.go`
- Test: `internal/repository/friendlink_test.go`

- [ ] **Step 1: Write failing repository tests**

Add tests for:

```go
func TestFriendLinkRepository_ListPublic_FiltersVisibleAndOrders(t *testing.T)
func TestFriendLinkRepository_ListAdmin_FiltersStatusWhenProvided(t *testing.T)
func TestFriendLinkRepository_Update_ReturnsNilWhenMissing(t *testing.T)
func TestFriendLinkRepository_Delete_SoftDeletes(t *testing.T)
```

- [ ] **Step 2: Run repository tests and verify RED**

Run: `go test ./internal/repository -run 'TestFriendLinkRepository' -count=1`

Expected: FAIL because repository methods are not implemented yet.

- [ ] **Step 3: Implement repository**

Implement interface:

```go
type FriendLinkRepository interface {
    ListPublic(offset, limit int) ([]model.FriendLink, int64, error)
    GetPublic(id uint) (*model.FriendLink, error)
    ListAdmin(offset, limit int, status *uint8) ([]model.FriendLink, int64, error)
    Create(link model.FriendLink) (*model.FriendLink, error)
    Update(id uint, data FriendLinkUpdateData) (*model.FriendLink, error)
    Delete(id uint) (*model.FriendLink, error)
}
```

Use `status=1` for public reads, `seq ASC, id DESC` ordering, and GORM soft delete for `Delete`.

- [ ] **Step 4: Run repository tests and verify GREEN**

Run: `go test ./internal/repository -run 'TestFriendLinkRepository' -count=1`

Expected: PASS.

## Task 3: Handler And Routes

**Files:**
- Modify: `internal/handler/friendlink.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/friendlink_test.go`

- [ ] **Step 1: Write failing handler tests**

Add tests for:

```go
func TestFriendLinkHandler_ListPublic_BindsQuery(t *testing.T)
func TestFriendLinkHandler_Create_InvalidJSONReturnsBadRequest(t *testing.T)
func TestFriendLinkHandler_GetPublic_NotFoundReturns404(t *testing.T)
func TestFriendLinkHandler_Update_BindsPathAndBody(t *testing.T)
```

- [ ] **Step 2: Run handler tests and verify RED**

Run: `go test ./internal/handler -run 'TestFriendLinkHandler' -count=1`

Expected: FAIL because handler methods are not implemented yet.

- [ ] **Step 3: Implement handler and route wiring**

Add public routes:

```go
r.GET("/friend-links", handlers.friendLink.ListPublic)
r.GET("/friend-links/:id", handlers.friendLink.GetPublic)
```

Add admin routes:

```go
admin.GET("/friend-links", handlers.friendLink.ListAdmin)
admin.POST("/friend-links", handlers.friendLink.Create)
admin.PUT("/friend-links/:id", handlers.friendLink.Update)
admin.DELETE("/friend-links/:id", handlers.friendLink.Delete)
```

Wire `repository.NewFriendLinkRepository(db)`, `service.NewFriendLinkService(repo, objectStore)`, and `handler.NewFriendLinkHandler(svc)`.

- [ ] **Step 4: Run handler tests and verify GREEN**

Run: `go test ./internal/handler -run 'TestFriendLinkHandler' -count=1`

Expected: PASS.

## Task 4: Swagger And Full Verification

**Files:**
- Modify: `docs/docs.go`
- Modify: `docs/swagger.json`
- Modify: `docs/swagger.yaml`

- [ ] **Step 1: Generate Swagger docs**

Run: `make swag`

Expected: exit 0 and generated docs include `/friend-links` and `/admin/friend-links`.

- [ ] **Step 2: Run targeted tests**

Run:

```bash
go test ./internal/service ./internal/repository ./internal/handler -run 'TestFriendLink' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 4: Inspect changed files**

Run: `git status --short && git diff --stat`

Expected: only friendlink implementation, router wiring, generated Swagger docs, and plan/spec files are changed.

## Self-Review

- Spec coverage: public list/detail, admin CRUD, DTO isolation, soft delete, status filtering, pagination, CDN resolution, Swagger, and tests are covered.
- Placeholder scan: no TBD/TODO/implement later steps remain.
- Type consistency: DTO names, service method names, repository method names, and route paths are consistent across tasks.
