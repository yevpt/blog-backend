package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
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

type stubTagService struct {
	createReq dto.TagCreateReq
	createRes *dto.TagItemResp
	createErr error
	addReq    dto.TagArticlesReq
	addRes    *dto.TagArticlesResp
	addErr    error
}

func (s *stubTagService) List() (*dto.TagListResp, error) {
	return &dto.TagListResp{}, nil
}

func (s *stubTagService) Get(id uint) (*dto.TagItemResp, error) {
	return &dto.TagItemResp{ID: id}, nil
}

func (s *stubTagService) ListArticles(id uint, req dto.ArticleListReq) (*dto.ArticlePageResp, error) {
	return &dto.ArticlePageResp{Page: req.Page, PageSize: req.PageSize}, nil
}

func (s *stubTagService) Create(req dto.TagCreateReq) (*dto.TagItemResp, error) {
	s.createReq = req
	return s.createRes, s.createErr
}

func (s *stubTagService) Update(id uint, req dto.TagUpdateReq) (*dto.TagItemResp, error) {
	return &dto.TagItemResp{ID: id}, nil
}

func (s *stubTagService) Delete(id uint) (*dto.TagItemResp, error) {
	return &dto.TagItemResp{ID: id}, nil
}

func (s *stubTagService) AddArticles(id uint, req dto.TagArticlesReq) (*dto.TagArticlesResp, error) {
	s.addReq = req
	return s.addRes, s.addErr
}

func (s *stubTagService) RemoveArticles(id uint, req dto.TagArticlesReq) (*dto.TagArticlesResp, error) {
	return &dto.TagArticlesResp{TagID: id}, nil
}

func newTagRouter(svc service.TagService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewTagHandler(svc)
	r.POST("/admin/tags", h.Create)
	r.POST("/admin/tags/:id/articles", h.AddArticles)
	return r
}

func TestTagHandler_Create_Success(t *testing.T) {
	seq := uint(0)
	stub := &stubTagService{
		createRes: &dto.TagItemResp{ID: 3, Name: "Go", Seq: seq},
	}
	r := newTagRouter(stub)
	body, _ := json.Marshal(dto.TagCreateReq{Name: "Go", Seq: &seq})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Go", stub.createReq.Name)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestTagHandler_AddArticles_BadRequest(t *testing.T) {
	stub := &stubTagService{addErr: service.ErrTagArticleRequired}
	r := newTagRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/tags/5/articles", bytes.NewReader([]byte(`{"article_ids":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestTagHandler_AddArticles_ServerError(t *testing.T) {
	stub := &stubTagService{addErr: errors.New("db down")}
	r := newTagRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/tags/5/articles", bytes.NewReader([]byte(`{"article_ids":[8]}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
