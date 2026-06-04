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
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubCommentService struct {
	createReq    dto.CommentCreateReq
	createUserID uint
	createResp   *dto.CommentItemResp
	createErr    error
	replyID      uint
	replyReq     dto.CommentReplyCreateReq
	replyUserID  uint
	replyResp    *dto.CommentReplyResp
	replyErr     error
}

func (s *stubCommentService) List(req dto.CommentListReq) (*dto.CommentPageResp, error) {
	return &dto.CommentPageResp{Page: 1, PageSize: 10}, nil
}

func (s *stubCommentService) Create(req dto.CommentCreateReq, userID uint) (*dto.CommentItemResp, error) {
	s.createReq = req
	s.createUserID = userID
	return s.createResp, s.createErr
}

func (s *stubCommentService) Reply(commentID uint, req dto.CommentReplyCreateReq, userID uint) (*dto.CommentReplyResp, error) {
	s.replyID = commentID
	s.replyReq = req
	s.replyUserID = userID
	return s.replyResp, s.replyErr
}

func (s *stubCommentService) DeleteComment(targetType string, commentID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	return &dto.CommentDeleteResp{ID: commentID}, nil
}

func (s *stubCommentService) DeleteReply(replyID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	return &dto.CommentDeleteResp{ID: replyID}, nil
}

func newCommentRouter(svc commentservice.CommentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := commenthandler.NewCommentHandler(svc)
	r.GET("/comments", h.List)
	r.POST("/comments", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7, Username: "vpt"})
		h.Create(c)
	})
	r.POST("/comments/:id/replies", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7, Username: "vpt"})
		h.Reply(c)
	})
	return r
}

func TestCommentHandler_Create_UsesClaimsUserID(t *testing.T) {
	stub := &stubCommentService{
		createResp: &dto.CommentItemResp{ID: 9, TargetType: "article", TargetID: 3, UserID: 7, Content: "好文章"},
	}
	r := newCommentRouter(stub)
	body, _ := json.Marshal(dto.CommentCreateReq{TargetType: "article", TargetID: 3, Content: "好文章"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.createUserID)
	assert.Equal(t, "article", stub.createReq.TargetType)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestCommentHandler_Reply_BindsCommentID(t *testing.T) {
	stub := &stubCommentService{
		replyResp: &dto.CommentReplyResp{ID: 12, CommentID: 9, FromUserID: 7, ToUserID: 8, Content: "收到"},
	}
	r := newCommentRouter(stub)
	body, _ := json.Marshal(dto.CommentReplyCreateReq{TargetType: "article", ParentReplyID: 11, Content: "收到"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/comments/9/replies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(9), stub.replyID)
	assert.Equal(t, uint(11), stub.replyReq.ParentReplyID)
	assert.Equal(t, uint(7), stub.replyUserID)
}

func TestCommentHandler_Create_BadRequestBusinessError(t *testing.T) {
	stub := &stubCommentService{createErr: commentservice.ErrCommentContentRequired}
	r := newCommentRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/comments", bytes.NewReader([]byte(`{"target_type":"article","target_id":3,"content":" "}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestCommentHandler_List_InvalidTargetTypeReturnsReadableMessage(t *testing.T) {
	r := newCommentRouter(&stubCommentService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/comments?target_type=post&target_id=3", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "评论目标类型只能是 article、moment、guestbook", resp.Message)
}
