package handler_test

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
	"github.com/vpt/blog-backend/internal/handler"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubFriendLinkService struct {
	listPublicReq dto.FriendLinkListReq
	getErr        error
	createReq     dto.FriendLinkCreateReq
	updateID      uint
	updateReq     dto.FriendLinkUpdateReq
}

func (s *stubFriendLinkService) ListPublic(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error) {
	s.listPublicReq = req
	return &dto.FriendLinkPageResp{Page: req.Page, PageSize: req.PageSize, List: []dto.FriendLinkItemResp{}}, nil
}

func (s *stubFriendLinkService) GetPublic(id uint) (*dto.FriendLinkItemResp, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &dto.FriendLinkItemResp{ID: id, Name: "友站"}, nil
}

func (s *stubFriendLinkService) ListAdmin(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error) {
	return &dto.FriendLinkPageResp{Page: req.Page, PageSize: req.PageSize, List: []dto.FriendLinkItemResp{}}, nil
}

func (s *stubFriendLinkService) Create(req dto.FriendLinkCreateReq) (*dto.FriendLinkItemResp, error) {
	s.createReq = req
	return &dto.FriendLinkItemResp{ID: 3, Name: req.Name}, nil
}

func (s *stubFriendLinkService) Update(id uint, req dto.FriendLinkUpdateReq) (*dto.FriendLinkItemResp, error) {
	s.updateID = id
	s.updateReq = req
	return &dto.FriendLinkItemResp{ID: id}, nil
}

func (s *stubFriendLinkService) Delete(id uint) (*dto.FriendLinkItemResp, error) {
	return &dto.FriendLinkItemResp{ID: id}, nil
}

func newFriendLinkRouter(svc service.FriendLinkService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewFriendLinkHandler(svc)
	r.GET("/friend-links", h.ListPublic)
	r.GET("/friend-links/:id", h.GetPublic)
	r.POST("/admin/friend-links", h.Create)
	r.PUT("/admin/friend-links/:id", h.Update)
	return r
}

func TestFriendLinkHandler_ListPublic_BindsQuery(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/friend-links?page=2&page_size=5", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, stub.listPublicReq.Page)
	assert.Equal(t, 5, stub.listPublicReq.PageSize)
}

func TestFriendLinkHandler_Create_InvalidJSONReturnsBadRequest(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/friend-links", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestFriendLinkHandler_GetPublic_NotFoundReturns404(t *testing.T) {
	stub := &stubFriendLinkService{getErr: service.ErrFriendLinkNotFound}
	r := newFriendLinkRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/friend-links/9", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFriendLinkHandler_Update_BindsPathAndBody(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)
	name := "友站"
	body, _ := json.Marshal(dto.FriendLinkUpdateReq{Name: &name})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/admin/friend-links/7", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.updateID)
	require.NotNil(t, stub.updateReq.Name)
	assert.Equal(t, name, *stub.updateReq.Name)
}
