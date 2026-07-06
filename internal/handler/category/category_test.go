package category_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/category"
	categoryservice "github.com/vpt/blog-backend/internal/service/category"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubCategoryService struct {
	createRes *dto.CategoryItemResp
	createErr error
	addRes    *dto.CategoryArticlesResp
	addErr    error
}

func (s *stubCategoryService) ListTabs() (*dto.CategoryTabsResp, error) {
	return &dto.CategoryTabsResp{}, nil
}

func (s *stubCategoryService) Create(_ context.Context, _ uint, req dto.CategoryCreateReq) (*dto.CategoryItemResp, error) {
	return s.createRes, s.createErr
}

func (s *stubCategoryService) Update(_ context.Context, _ uint, id uint, req dto.CategoryUpdateReq) (*dto.CategoryItemResp, error) {
	return &dto.CategoryItemResp{ID: id}, nil
}

func (s *stubCategoryService) Delete(_ context.Context, id uint) (*dto.CategoryItemResp, error) {
	return &dto.CategoryItemResp{ID: id}, nil
}

func (s *stubCategoryService) AddArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error) {
	return s.addRes, s.addErr
}

func (s *stubCategoryService) RemoveArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error) {
	return &dto.CategoryArticlesResp{CategoryID: id}, nil
}

func (s *stubCategoryService) UploadIcon(_ context.Context, _ uint, _ string, _ []byte) (*dto.CategoryAssetUploadResp, error) {
	return nil, nil
}

func (s *stubCategoryService) UploadCover(_ context.Context, _ uint, _ string, _ []byte) (*dto.CategoryAssetUploadResp, error) {
	return nil, nil
}

// setAdminClaims 在 gin 上下文中写入 admin JWT claims。
func setAdminClaims(c *gin.Context) {
	jwt.SetClaims(c, &jwt.Claims{UserId: 1})
}

func newCategoryRouter(svc categoryservice.CategoryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := category.NewCategoryHandler(svc)
	r.POST("/admin/categories", func(c *gin.Context) {
		setAdminClaims(c)
		h.Create(c)
	})
	r.POST("/admin/categories/:id/articles", h.AddArticles)
	r.POST("/admin/categories/uploads/icon", func(c *gin.Context) {
		setAdminClaims(c)
		h.UploadIcon(c)
	})
	r.POST("/admin/categories/uploads/cover", func(c *gin.Context) {
		setAdminClaims(c)
		h.UploadCover(c)
	})
	return r
}

func TestCategoryHandler_Create_Success(t *testing.T) {
	seq := uint(0)
	stub := &stubCategoryService{
		createRes: &dto.CategoryItemResp{ID: 3, Name: "编程", Seq: seq},
	}
	r := newCategoryRouter(stub)
	body, _ := json.Marshal(dto.CategoryCreateReq{
		Name: "编程",
		Seq:  &seq,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestCategoryHandler_Create_OptionalFieldsOmitted_Succeeds(t *testing.T) {
	// 图标、描述、封面全部不传也应创建成功
	seq := uint(1)
	stub := &stubCategoryService{
		createRes: &dto.CategoryItemResp{ID: 5, Name: "日记", Seq: seq},
	}
	r := newCategoryRouter(stub)
	body, _ := json.Marshal(dto.CategoryCreateReq{
		Name: "日记",
		Seq:  &seq,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestCategoryHandler_AddArticles_BadRequest(t *testing.T) {
	stub := &stubCategoryService{addErr: categoryservice.ErrCategoryArticleRequired}
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

func TestCategoryHandler_UploadIcon_MissingFile(t *testing.T) {
	// 无文件字段应返回 400
	stub := &stubCategoryService{}
	r := newCategoryRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories/uploads/icon", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=----test")
	r.ServeHTTP(w, req)

	// 缺 file 字段 -> 400
	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestCategoryHandler_UploadCover_MissingFile(t *testing.T) {
	stub := &stubCategoryService{}
	r := newCategoryRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories/uploads/cover", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=----test")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}
