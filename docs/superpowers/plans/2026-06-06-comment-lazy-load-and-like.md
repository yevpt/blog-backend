# Comment Lazy Load And Like Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将评论列表改造成回复懒加载，并为评论与回复补齐点赞查询和切换能力。

**Architecture:** 保持现有 `handler -> service -> repository` 分层，公开查询接口接入 `OptionalAuth` 以补充 `is_liked`。一级评论列表仅返回计数与点赞摘要，回复列表通过独立分页接口查询，点赞切换继续复用 `user_like` 软删除恢复模式。

**Tech Stack:** Go 1.25+, Gin, GORM/MySQL, swaggo/swag, testify, sqlmock

---

### Task 1: 调整评论 DTO 与 service 入口

**Files:**
- Modify: `internal/dto/comment.go`
- Modify: `internal/service/comment/comment.go`
- Test: `internal/service/comment/comment_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCommentService_ListReplies_UsesViewerAndPaging(t *testing.T) {}

func TestCommentService_ToggleCommentLike_InvalidID(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/comment -run 'TestCommentService_(ListReplies_UsesViewerAndPaging|ToggleCommentLike_InvalidID)'`
Expected: FAIL with missing methods or mismatched interfaces

- [ ] **Step 3: Write minimal implementation**

```go
type CommentReplyListReq struct {
	Page int `form:"page"`
	PageSize int `form:"page_size"`
}

type CommentLikeResp struct {
	IsLiked bool `json:"is_liked"`
	LikeCount int64 `json:"like_count"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/comment -run 'TestCommentService_(ListReplies_UsesViewerAndPaging|ToggleCommentLike_InvalidID)'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/dto/comment.go internal/service/comment/comment.go internal/service/comment/comment_test.go
git commit -m "feat: 补充评论懒加载 DTO 与服务接口"
```

### Task 2: 仓储层实现回复分页与点赞聚合

**Files:**
- Modify: `internal/repository/comment/comment.go`
- Modify: `internal/repository/comment/query.go`
- Modify: `internal/repository/comment/reply.go`
- Modify: `internal/repository/comment/mutation.go`
- Add: `internal/repository/comment/like.go`
- Test: `internal/repository/comment/comment_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCommentRepository_List_OnlyReturnsReplyCountAndLikeState(t *testing.T) {}

func TestCommentRepository_ListReplies_ReturnsPagedRepliesWithLikeState(t *testing.T) {}

func TestCommentRepository_ToggleCommentLike_ReturnsLatestState(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/comment -run 'TestCommentRepository_(List_OnlyReturnsReplyCountAndLikeState|ListReplies_ReturnsPagedRepliesWithLikeState|ToggleCommentLike_ReturnsLatestState)'`
Expected: FAIL with missing repository methods or wrong aggregate fields

- [ ] **Step 3: Write minimal implementation**

```go
type CommentAggregate struct {
	Comment CommentRecord
	User *model.User
	ReplyCount int64
	LikeCount int64
	IsLiked bool
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/repository/comment -run 'TestCommentRepository_(List_OnlyReturnsReplyCountAndLikeState|ListReplies_ReturnsPagedRepliesWithLikeState|ToggleCommentLike_ReturnsLatestState)'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/comment/comment.go internal/repository/comment/query.go internal/repository/comment/reply.go internal/repository/comment/mutation.go internal/repository/comment/like.go internal/repository/comment/comment_test.go
git commit -m "feat: 支持评论回复分页与点赞仓储"
```

### Task 3: HTTP 接口与路由接线

**Files:**
- Modify: `internal/handler/comment/binding.go`
- Modify: `internal/handler/comment/query.go`
- Modify: `internal/handler/comment/mutation.go`
- Modify: `internal/handler/comment/comment_test.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCommentHandler_ListReplies_BindsCommentIDAndPaging(t *testing.T) {}

func TestCommentHandler_ToggleCommentLike_BindsIDAndUser(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/comment -run 'TestCommentHandler_(ListReplies_BindsCommentIDAndPaging|ToggleCommentLike_BindsIDAndUser)'`
Expected: FAIL with missing routes or handler methods

- [ ] **Step 3: Write minimal implementation**

```go
r.GET("/comments", middleware.OptionalAuth(jwtManager), handlers.comment.List)
r.GET("/comments/:id/replies", middleware.OptionalAuth(jwtManager), handlers.comment.ListReplies)
authed.POST("/comments/:id/like", handlers.comment.ToggleLike)
authed.POST("/comments/:id/replies/:replyId/like", handlers.comment.ToggleReplyLike)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/comment -run 'TestCommentHandler_(ListReplies_BindsCommentIDAndPaging|ToggleCommentLike_BindsIDAndUser)'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/handler/comment/binding.go internal/handler/comment/query.go internal/handler/comment/mutation.go internal/handler/comment/comment_test.go internal/router/router.go
git commit -m "feat: 接入评论回复懒加载与点赞接口"
```

### Task 4: Swagger 与回归验证

**Files:**
- Modify: `docs/docs.go`
- Modify: `docs/swagger.json`
- Modify: `docs/swagger.yaml`

- [ ] **Step 1: Regenerate swagger**

Run: `make swag`
Expected: `docs/swagger.yaml` 与 `docs/swagger.json` 出现新的评论回复分页与点赞接口

- [ ] **Step 2: Run focused tests**

Run: `go test ./internal/service/comment ./internal/repository/comment ./internal/handler/comment`
Expected: PASS

- [ ] **Step 3: Run broader regression**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add docs/docs.go docs/swagger.json docs/swagger.yaml
git commit -m "docs: 更新评论接口 swagger"
```
