package comment_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/dto"
	commenthandler "github.com/vpt/blog-backend/internal/handler/comment"
	"github.com/vpt/blog-backend/internal/middleware"
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/response"
)

type lifecycleCommentService struct {
	stubCommentService
	state string
}

func (s *lifecycleCommentService) Create(context.Context, string, uint, dto.CommentCreateReq, uint) (*dto.CommentItemResp, error) {
	s.state = "low_pending"
	return &dto.CommentItemResp{
		ID: 9, TargetType: "article", TargetID: 3, UserID: 7, Content: "低风险正文",
		Moderation: dto.ModerationView{
			Notice: "发布成功，内容会被审核。", PublicState: "visible", DisplayVersion: "pending",
			HasPendingRevision: true,
		},
	}, nil
}

func (s *lifecycleCommentService) List(context.Context, string, uint, dto.CommentListReq, *uint) (*dto.CommentPageResp, error) {
	if s.state == "deleted" || s.state == "" {
		return &dto.CommentPageResp{Page: 1, PageSize: 10, List: []dto.CommentItemResp{}}, nil
	}
	item := dto.CommentItemResp{ID: 9, TargetType: "article", TargetID: 3, UserID: 7}
	if s.state == "low_pending" {
		item.Content = "低风险正文"
		item.Moderation = dto.ModerationView{PublicState: "visible", DisplayVersion: "pending", HasPendingRevision: true}
	} else {
		item.Moderation = dto.ModerationView{PublicState: "placeholder", DisplayVersion: "none", HasPendingRevision: true}
	}
	return &dto.CommentPageResp{Total: 1, Pages: 1, Page: 1, PageSize: 10, List: []dto.CommentItemResp{item}}, nil
}

func (s *lifecycleCommentService) Reply(context.Context, string, uint, dto.CommentReplyCreateReq, uint) (*dto.CommentReplyResp, error) {
	return nil, moderationservice.ErrInteractionNotAllowed
}

func (s *lifecycleCommentService) ToggleLike(context.Context, string, uint, uint) (*dto.CommentLikeResp, error) {
	return nil, moderationservice.ErrInteractionNotAllowed
}

func (s *lifecycleCommentService) EditComment(context.Context, string, uint, dto.CommentCreateReq, uint, []string) (*dto.CommentItemResp, error) {
	if s.state == "deleted" {
		return nil, moderationservice.ErrAlreadyDeleted
	}
	s.state = "medium_pending"
	return &dto.CommentItemResp{
		ID: 9, Moderation: dto.ModerationView{
			Notice: "内容已提交，等待人工审核。", PublicState: "placeholder", DisplayVersion: "none",
			HasPendingRevision: true,
		},
	}, nil
}

func (s *lifecycleCommentService) DeleteComment(context.Context, string, uint, uint, []string) (*dto.CommentDeleteResp, error) {
	s.state = "deleted"
	return &dto.CommentDeleteResp{ID: 9}, nil
}

var _ commentservice.CommentService = (*lifecycleCommentService)(nil)

func TestCommentHTTPLifecycleHonorsModerationStates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &lifecycleCommentService{}
	handler := commenthandler.NewCommentHandler(service, true)
	router := gin.New()
	authenticated := func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1})
			next(c)
		}
	}
	router.GET("/articles/:id/comments", handler.ListArticle)
	router.POST("/articles/:id/comments", authenticated(handler.CreateArticle))
	router.POST("/articles/comments/:id/replies", authenticated(handler.ReplyArticle))
	router.POST("/articles/comments/:id/like", authenticated(handler.ToggleArticleLike))
	router.PATCH("/articles/comments/:id", authenticated(handler.EditArticle))
	router.DELETE("/articles/comments/:id", authenticated(handler.DeleteArticle))

	created := lifecycleRequest(router, http.MethodPost, "/articles/3/comments", `{"content":"低风险正文"}`, "create-1")
	assert.Equal(t, http.StatusOK, created.Code)
	assert.Contains(t, created.Body.String(), "发布成功，内容会被审核。")

	publicLow := lifecycleRequest(router, http.MethodGet, "/articles/3/comments", "", "")
	assert.Contains(t, publicLow.Body.String(), "低风险正文")
	assert.Contains(t, publicLow.Body.String(), `"display_version":"pending"`)

	reply := lifecycleRequest(router, http.MethodPost, "/articles/comments/9/replies", `{"content":"回复"}`, "reply-1")
	assert.Equal(t, http.StatusConflict, reply.Code)
	assert.Contains(t, reply.Body.String(), response.CodeContentPendingNoInteraction)
	liked := lifecycleRequest(router, http.MethodPost, "/articles/comments/9/like", "", "")
	assert.Equal(t, http.StatusConflict, liked.Code)

	edited := lifecycleRequest(router, http.MethodPatch, "/articles/comments/9", `{"content":"中风险正文"}`, "edit-1")
	assert.Equal(t, http.StatusOK, edited.Code)
	assert.NotContains(t, edited.Body.String(), "中风险正文")
	publicMedium := lifecycleRequest(router, http.MethodGet, "/articles/3/comments", "", "")
	assert.Contains(t, publicMedium.Body.String(), `"public_state":"placeholder"`)
	assert.NotContains(t, publicMedium.Body.String(), "中风险正文")

	deleted := lifecycleRequest(router, http.MethodDelete, "/articles/comments/9", "", "")
	assert.Equal(t, http.StatusOK, deleted.Code)
	editDeleted := lifecycleRequest(router, http.MethodPatch, "/articles/comments/9", `{"content":"复活"}`, "edit-2")
	assert.Equal(t, http.StatusConflict, editDeleted.Code)
	assert.Contains(t, editDeleted.Body.String(), response.CodeContentAlreadyDeleted)
}

func lifecycleRequest(router http.Handler, method string, path string, body string, idempotencyKey string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}
