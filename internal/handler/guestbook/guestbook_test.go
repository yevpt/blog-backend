package guestbook_test

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
	guestbookhandler "github.com/vpt/blog-backend/internal/handler/guestbook"
	"github.com/vpt/blog-backend/internal/middleware"
	guestbookservice "github.com/vpt/blog-backend/internal/service/guestbook"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
	"github.com/vpt/blog-backend/pkg/roles"
)

type stubGuestbookService struct {
	listReq      dto.GuestbookListReq
	listViewerID *uint
	listResp     *dto.GuestbookPageResp
	listErr      error

	createReq    dto.GuestbookCreateReq
	createUserID uint
	createResp   *dto.GuestbookItemResp
	createErr    error

	likeID     uint
	likeUserID uint
	likeResp   *dto.GuestbookLikeResp
	likeErr    error

	deleteID     uint
	deleteUserID uint
	deleteRoles  []string
	deleteResp   *dto.GuestbookDeleteResp
	deleteErr    error
}

func (s *stubGuestbookService) List(req dto.GuestbookListReq, viewerID *uint) (*dto.GuestbookPageResp, error) {
	s.listReq = req
	s.listViewerID = viewerID
	return s.listResp, s.listErr
}

func (s *stubGuestbookService) Create(req dto.GuestbookCreateReq, fromUserID uint) (*dto.GuestbookItemResp, error) {
	s.createReq = req
	s.createUserID = fromUserID
	return s.createResp, s.createErr
}

func (s *stubGuestbookService) ToggleLike(id uint, userID uint) (*dto.GuestbookLikeResp, error) {
	s.likeID = id
	s.likeUserID = userID
	return s.likeResp, s.likeErr
}

func (s *stubGuestbookService) Delete(id uint, userID uint, roleNames []string) (*dto.GuestbookDeleteResp, error) {
	s.deleteID = id
	s.deleteUserID = userID
	s.deleteRoles = roleNames
	return s.deleteResp, s.deleteErr
}

func newGuestbookRouter(svc guestbookservice.GuestbookService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := guestbookhandler.NewGuestbookHandler(svc)
	r.GET("/guestbook", h.List)
	r.POST("/guestbook", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1})
		h.Create(c)
	})
	r.POST("/guestbook/:id/like", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1})
		h.ToggleLike(c)
	})
	r.DELETE("/guestbook/:id", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1, Roles: []string{roles.AdminRole}})
		h.Delete(c)
	})
	return r
}

func TestGuestbookHandler_List_AllowsMissingOwnerUserID(t *testing.T) {
	stub := &stubGuestbookService{
		listResp: &dto.GuestbookPageResp{Page: 1, PageSize: 10},
	}
	r := newGuestbookRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/guestbook?page=1&page_size=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(0), stub.listReq.OwnerUserID)
	assert.Nil(t, stub.listViewerID)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestGuestbookHandler_Create_UsesClaimsUserID(t *testing.T) {
	stub := &stubGuestbookService{
		createResp: &dto.GuestbookItemResp{ID: 9, OwnerUserID: 1, FromUserID: 7, Content: "你好"},
	}
	r := newGuestbookRouter(stub)
	body, _ := json.Marshal(dto.GuestbookCreateReq{Content: "你好"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/guestbook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.createUserID)
	assert.Equal(t, uint(0), stub.createReq.OwnerUserID)
	assert.Equal(t, "你好", stub.createReq.Content)
}

func TestGuestbookHandler_ToggleLike_BindsID(t *testing.T) {
	stub := &stubGuestbookService{
		likeResp: &dto.GuestbookLikeResp{ID: 9, IsLiked: true, LikeCount: 1},
	}
	r := newGuestbookRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/guestbook/9/like", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(9), stub.likeID)
	assert.Equal(t, uint(7), stub.likeUserID)
}

func TestGuestbookHandler_Delete_ForwardsRoles(t *testing.T) {
	stub := &stubGuestbookService{
		deleteResp: &dto.GuestbookDeleteResp{ID: 9},
	}
	r := newGuestbookRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/guestbook/9", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(9), stub.deleteID)
	assert.Equal(t, uint(7), stub.deleteUserID)
	assert.Equal(t, []string{roles.AdminRole}, stub.deleteRoles)
}
