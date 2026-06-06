package comment_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	commenthandler "github.com/vpt/blog-backend/internal/handler/comment"
	"github.com/vpt/blog-backend/internal/middleware"
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubCommentService struct {
	listTargetType        string
	listTargetID          uint
	listReq               dto.CommentListReq
	listViewerID          *uint
	listResp              *dto.CommentPageResp
	listErr               error
	createTargetType      string
	createTargetID        uint
	createReq             dto.CommentCreateReq
	createUserID          uint
	createResp            *dto.CommentItemResp
	createErr             error
	listRepliesTargetType string
	listRepliesCommentID  uint
	listRepliesReq        dto.CommentReplyListReq
	listRepliesViewerID   *uint
	listRepliesResp       *dto.CommentReplyPageResp
	listRepliesErr        error
	replyTargetType       string
	replyCommentID        uint
	replyReq              dto.CommentReplyCreateReq
	replyUserID           uint
	replyResp             *dto.CommentReplyResp
	replyErr              error
	toggleLikeTargetType  string
	toggleLikeCommentID   uint
	toggleLikeUserID      uint
	toggleLikeResp        *dto.CommentLikeResp
	toggleLikeErr         error
}

func (s *stubCommentService) List(targetType string, targetID uint, req dto.CommentListReq, viewerID *uint) (*dto.CommentPageResp, error) {
	s.listTargetType = targetType
	s.listTargetID = targetID
	s.listReq = req
	s.listViewerID = viewerID
	return s.listResp, s.listErr
}

func (s *stubCommentService) Create(targetType string, targetID uint, req dto.CommentCreateReq, userID uint) (*dto.CommentItemResp, error) {
	s.createTargetType = targetType
	s.createTargetID = targetID
	s.createReq = req
	s.createUserID = userID
	return s.createResp, s.createErr
}

func (s *stubCommentService) ListReplies(targetType string, commentID uint, req dto.CommentReplyListReq, viewerID *uint) (*dto.CommentReplyPageResp, error) {
	s.listRepliesTargetType = targetType
	s.listRepliesCommentID = commentID
	s.listRepliesReq = req
	s.listRepliesViewerID = viewerID
	return s.listRepliesResp, s.listRepliesErr
}

func (s *stubCommentService) Reply(targetType string, commentID uint, req dto.CommentReplyCreateReq, userID uint) (*dto.CommentReplyResp, error) {
	s.replyTargetType = targetType
	s.replyCommentID = commentID
	s.replyReq = req
	s.replyUserID = userID
	return s.replyResp, s.replyErr
}

func (s *stubCommentService) ToggleLike(targetType string, commentID uint, userID uint) (*dto.CommentLikeResp, error) {
	s.toggleLikeTargetType = targetType
	s.toggleLikeCommentID = commentID
	s.toggleLikeUserID = userID
	return s.toggleLikeResp, s.toggleLikeErr
}

func (s *stubCommentService) ToggleReplyLike(targetType string, replyID uint, userID uint) (*dto.CommentLikeResp, error) {
	return &dto.CommentLikeResp{IsLiked: true, LikeCount: 1}, nil
}

func (s *stubCommentService) DeleteComment(targetType string, commentID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	return &dto.CommentDeleteResp{ID: commentID}, nil
}

func (s *stubCommentService) DeleteReply(targetType string, replyID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	return &dto.CommentDeleteResp{ID: replyID}, nil
}

func newCommentRouter(svc commentservice.CommentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := commenthandler.NewCommentHandler(svc)
	r.GET("/articles/:id/comments", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 9})
		h.ListArticle(c)
	})
	r.POST("/articles/:id/comments", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "vpt", Status: 1})
		h.CreateArticle(c)
	})
	r.GET("/articles/comments/:id/replies", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 9})
		h.ListArticleReplies(c)
	})
	r.POST("/articles/comments/:id/replies", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "vpt", Status: 1})
		h.ReplyArticle(c)
	})
	r.POST("/articles/comments/:id/like", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "vpt", Status: 1})
		h.ToggleArticleLike(c)
	})
	return r
}

func TestCommentHandler_ListArticle_UsesPathTargetAndOptionalViewer(t *testing.T) {
	stub := &stubCommentService{
		listResp: &dto.CommentPageResp{Page: 1, PageSize: 10},
	}
	r := newCommentRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles/3/comments?page=2&page_size=5", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "article", stub.listTargetType)
	assert.Equal(t, uint(3), stub.listTargetID)
	assert.Equal(t, 2, stub.listReq.Page)
	assert.Equal(t, 5, stub.listReq.PageSize)
	require.NotNil(t, stub.listViewerID)
	assert.Equal(t, uint(9), *stub.listViewerID)
}

func TestCommentHandler_CreateArticle_UsesClaimsUserID(t *testing.T) {
	stub := &stubCommentService{
		createResp: &dto.CommentItemResp{ID: 9, TargetType: "article", TargetID: 3, UserID: 7, Content: "好文章"},
	}
	r := newCommentRouter(stub)
	body, _ := json.Marshal(dto.CommentCreateReq{Content: "好文章"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/articles/3/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "article", stub.createTargetType)
	assert.Equal(t, uint(3), stub.createTargetID)
	assert.Equal(t, uint(7), stub.createUserID)
	assert.Equal(t, "好文章", stub.createReq.Content)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestCommentHandler_ListArticleReplies_BindsCommentIDAndPaging(t *testing.T) {
	stub := &stubCommentService{
		listRepliesResp: &dto.CommentReplyPageResp{Page: 1, PageSize: 10},
	}
	r := newCommentRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles/comments/9/replies?page=3&page_size=6", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "article", stub.listRepliesTargetType)
	assert.Equal(t, uint(9), stub.listRepliesCommentID)
	assert.Equal(t, 3, stub.listRepliesReq.Page)
	assert.Equal(t, 6, stub.listRepliesReq.PageSize)
}

func TestCommentHandler_ReplyArticle_BindsCommentID(t *testing.T) {
	stub := &stubCommentService{
		replyResp: &dto.CommentReplyResp{ID: 12, CommentID: 9, FromUserID: 7, ToUserID: 8, Content: "收到"},
	}
	r := newCommentRouter(stub)
	body, _ := json.Marshal(dto.CommentReplyCreateReq{ParentReplyID: 11, Content: "收到"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/articles/comments/9/replies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "article", stub.replyTargetType)
	assert.Equal(t, uint(9), stub.replyCommentID)
	assert.Equal(t, uint(11), stub.replyReq.ParentReplyID)
	assert.Equal(t, uint(7), stub.replyUserID)
}

func TestCommentHandler_ToggleArticleLike_BindsIDAndUser(t *testing.T) {
	stub := &stubCommentService{
		toggleLikeResp: &dto.CommentLikeResp{IsLiked: true, LikeCount: 3},
	}
	r := newCommentRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/articles/comments/12/like", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "article", stub.toggleLikeTargetType)
	assert.Equal(t, uint(12), stub.toggleLikeCommentID)
	assert.Equal(t, uint(7), stub.toggleLikeUserID)
}
