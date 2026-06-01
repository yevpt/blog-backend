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

type stubCategoryService struct {
	createReq dto.CategoryCreateReq
	createRes *dto.CategoryItemResp
	createErr error
	addReq    dto.CategoryArticlesReq
	addRes    *dto.CategoryArticlesResp
	addErr    error
}

func (s *stubCategoryService) ListTabs() (*dto.CategoryTabsResp, error) {
	return &dto.CategoryTabsResp{}, nil
}

func (s *stubCategoryService) Create(req dto.CategoryCreateReq) (*dto.CategoryItemResp, error) {
	s.createReq = req
	return s.createRes, s.createErr
}

func (s *stubCategoryService) Update(id uint, req dto.CategoryUpdateReq) (*dto.CategoryItemResp, error) {
	return &dto.CategoryItemResp{ID: id}, nil
}

func (s *stubCategoryService) Delete(id uint) (*dto.CategoryItemResp, error) {
	return &dto.CategoryItemResp{ID: id}, nil
}

func (s *stubCategoryService) AddArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error) {
	s.addReq = req
	return s.addRes, s.addErr
}

func (s *stubCategoryService) RemoveArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error) {
	return &dto.CategoryArticlesResp{CategoryID: id}, nil
}

func newCategoryRouter(svc service.CategoryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewCategoryHandler(svc)
	r.POST("/admin/categories", h.Create)
	r.POST("/admin/categories/:id/articles", h.AddArticles)
	return r
}

func TestCategoryHandler_Create_Success(t *testing.T) {
	seq := uint(0)
	stub := &stubCategoryService{
		createRes: &dto.CategoryItemResp{ID: 3, Name: "编程", Seq: seq},
	}
	r := newCategoryRouter(stub)
	body, _ := json.Marshal(dto.CategoryCreateReq{
		Name:        "编程",
		Seq:         &seq,
		Icon:        "icon",
		Description: "desc",
		CoverImgUrl: "cover",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "编程", stub.createReq.Name)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestCategoryHandler_AddArticles_BadRequest(t *testing.T) {
	stub := &stubCategoryService{addErr: service.ErrCategoryArticleRequired}
	r := newCategoryRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories/5/articles", bytes.NewReader([]byte(`{"article_ids":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestCategoryHandler_AddArticles_ServerError(t *testing.T) {
	stub := &stubCategoryService{addErr: errors.New("db down")}
	r := newCategoryRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories/5/articles", bytes.NewReader([]byte(`{"article_ids":[8]}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
