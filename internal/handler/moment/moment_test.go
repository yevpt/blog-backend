package moment_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	momenthandler "github.com/vpt/blog-backend/internal/handler/moment"
	"github.com/vpt/blog-backend/internal/middleware"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
	"github.com/vpt/blog-backend/pkg/roles"
)

type stubMomentService struct {
	listReq      dto.MomentListReq
	listViewerID *uint
	listResp     *dto.MomentPageResp
	listErr      error

	saveReq    dto.MomentSaveReq
	saveUserID uint
	saveRoles  []string
	saveResp   *dto.MomentItemResp
	saveErr    error

	likeID     uint
	likeUserID uint
	likeResp   *dto.MomentItemResp
	likeErr    error

	deleteID     uint
	deleteUserID uint
	deleteRoles  []string
	deleteResp   *dto.MomentDeleteResp
	deleteErr    error
}

func (s *stubMomentService) List(req dto.MomentListReq, viewerID *uint) (*dto.MomentPageResp, error) {
	s.listReq = req
	s.listViewerID = viewerID
	return s.listResp, s.listErr
}

func (s *stubMomentService) GetDetail(uint, *uint) (*dto.MomentItemResp, error) {
	return &dto.MomentItemResp{ID: 9}, nil
}

func (s *stubMomentService) Save(req dto.MomentSaveReq, operatorID uint, roleNames []string) (*dto.MomentItemResp, error) {
	s.saveReq = req
	s.saveUserID = operatorID
	s.saveRoles = roleNames
	return s.saveResp, s.saveErr
}

func (s *stubMomentService) Delete(id uint, operatorID uint, roleNames []string) (*dto.MomentDeleteResp, error) {
	s.deleteID = id
	s.deleteUserID = operatorID
	s.deleteRoles = roleNames
	return s.deleteResp, s.deleteErr
}

func (s *stubMomentService) SetTop(uint, uint, []string) (*dto.MomentTopResp, error) {
	return &dto.MomentTopResp{ID: 9, IsTop: true}, nil
}

func (s *stubMomentService) RemoveTop(uint, uint, []string) (*dto.MomentTopResp, error) {
	return &dto.MomentTopResp{ID: 9, IsTop: false}, nil
}

func (s *stubMomentService) View(uint, string) (*dto.MomentViewResp, error) {
	return &dto.MomentViewResp{ID: 9, ViewCount: 1}, nil
}

func (s *stubMomentService) IsLiked(uint, uint) (*dto.MomentLikeResp, error) {
	return &dto.MomentLikeResp{IsLiked: true, LikeCount: 1}, nil
}

func (s *stubMomentService) ToggleLike(id uint, userID uint) (*dto.MomentItemResp, error) {
	s.likeID = id
	s.likeUserID = userID
	return s.likeResp, s.likeErr
}

func newMomentRouter(svc momentservice.MomentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := momenthandler.NewMomentHandler(svc)
	r.GET("/moments", h.List)
	r.POST("/moments", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1, Roles: []string{roles.AdminRole}})
		h.Save(c)
	})
	r.POST("/moments/:id/like", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1})
		h.ToggleLike(c)
	})
	r.DELETE("/moments/:id", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1, Roles: []string{roles.AdminRole}})
		h.Delete(c)
	})
	return r
}

func TestMomentHandler_List_AllowsOptionalAuth(t *testing.T) {
	stub := &stubMomentService{listResp: &dto.MomentPageResp{Page: 1, PageSize: 10}}
	r := newMomentRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/moments?page=1&page_size=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, stub.listViewerID)
	assert.Equal(t, 1, stub.listReq.Page)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestMomentHandler_Save_UsesClaimsUserAndRoles(t *testing.T) {
	stub := &stubMomentService{saveResp: &dto.MomentItemResp{ID: 9, UserID: 7, Content: "风"}}
	r := newMomentRouter(stub)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("content", "风"))
	require.NoError(t, writer.WriteField("status", "1"))
	require.NoError(t, writer.WriteField("comment_status", "1"))
	require.NoError(t, writer.WriteField("image_urls", "https://cdn.example.com/blog/moments/old.jpg?sign=1"))
	require.NoError(t, writer.WriteField("image_order", "file:0"))
	require.NoError(t, writer.WriteField("image_order", "url:0"))
	file, err := writer.CreateFormFile("images", "cat.jpg")
	require.NoError(t, err)
	_, err = file.Write([]byte("cat image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/moments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.saveUserID)
	assert.Equal(t, []string{roles.AdminRole}, stub.saveRoles)
	assert.Equal(t, "风", stub.saveReq.Content)
	assert.Equal(t, []string{"https://cdn.example.com/blog/moments/old.jpg?sign=1"}, stub.saveReq.ImageURLs)
	assert.Equal(t, []string{"file:0", "url:0"}, stub.saveReq.ImageOrder)
	require.Len(t, stub.saveReq.ImageFiles, 1)
	assert.Equal(t, "cat.jpg", stub.saveReq.ImageFiles[0].Name)
	assert.Equal(t, []byte("cat image"), stub.saveReq.ImageFiles[0].Data)
}

func TestMomentHandler_Save_RejectsImageOverOneMBBeforeService(t *testing.T) {
	stub := &stubMomentService{saveResp: &dto.MomentItemResp{ID: 9}}
	r := newMomentRouter(stub)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("content", "风"))
	require.NoError(t, writer.WriteField("status", "1"))
	require.NoError(t, writer.WriteField("comment_status", "1"))
	file, err := writer.CreateFormFile("images", "big.jpg")
	require.NoError(t, err)
	_, err = file.Write(bytes.Repeat([]byte{1}, 1024*1024+1))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/moments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Zero(t, stub.saveUserID)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestMomentHandler_ToggleLike_BindsIDAndUser(t *testing.T) {
	stub := &stubMomentService{likeResp: &dto.MomentItemResp{ID: 9, IsLiked: true}}
	r := newMomentRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/moments/9/like", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(9), stub.likeID)
	assert.Equal(t, uint(7), stub.likeUserID)
}

func TestMomentHandler_Delete_ForwardsRoles(t *testing.T) {
	stub := &stubMomentService{deleteResp: &dto.MomentDeleteResp{ID: 9}}
	r := newMomentRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/moments/9", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(9), stub.deleteID)
	assert.Equal(t, uint(7), stub.deleteUserID)
	assert.Equal(t, []string{roles.AdminRole}, stub.deleteRoles)
}
