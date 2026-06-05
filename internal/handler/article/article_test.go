package article_test

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
	articlehandler "github.com/vpt/blog-backend/internal/handler/article"
	articleservice "github.com/vpt/blog-backend/internal/service/article"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubArticleService struct {
	listReq    dto.ArticleListReq
	listViewer *uint
	listResp   *dto.ArticlePageResp
	listErr    error
	detailResp *dto.ArticleDetailResp
	detailErr  error
	saveReq    dto.ArticleSaveReq
	saveUserID uint
	saveResp   *dto.ArticleDetailResp
	saveErr    error
	likeResp   *dto.ArticleLikeResp
	likeErr    error
}

func (s *stubArticleService) ListIDs() (*dto.ArticleIDsResp, error) {
	return &dto.ArticleIDsResp{}, nil
}
func (s *stubArticleService) ListPublic(req dto.ArticleListReq, viewerID *uint) (*dto.ArticlePageResp, error) {
	s.listReq = req
	s.listViewer = viewerID
	return s.listResp, s.listErr
}
func (s *stubArticleService) GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	return s.detailResp, s.detailErr
}
func (s *stubArticleService) GetAdminDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	return s.detailResp, s.detailErr
}
func (s *stubArticleService) Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error) {
	s.saveReq = req
	s.saveUserID = authorID
	return s.saveResp, s.saveErr
}
func (s *stubArticleService) Delete(id uint) (*dto.ArticleDetailResp, error) {
	return s.detailResp, s.detailErr
}
func (s *stubArticleService) Read(id uint) (*dto.ArticleReadResp, error) {
	return &dto.ArticleReadResp{ID: id, ReadCount: 2}, nil
}
func (s *stubArticleService) IsLiked(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	return &dto.ArticleLikeResp{IsLiked: true, LikeCount: 3}, nil
}
func (s *stubArticleService) ToggleLike(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	return s.likeResp, s.likeErr
}

func newArticleRouter(svc articleservice.ArticleService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := articlehandler.NewArticleHandler(svc)
	r.GET("/articles", h.ListPublic)
	r.GET("/authed/articles", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 9})
		h.ListPublic(c)
	})
	r.GET("/articles/:id", h.GetPublicDetail)
	r.POST("/articles/:id/like", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 9})
		h.ToggleLike(c)
	})
	r.POST("/admin/articles", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		h.Save(c)
	})
	return r
}

func TestArticleHandler_ListPublic_Success(t *testing.T) {
	stub := &stubArticleService{
		listResp: &dto.ArticlePageResp{Page: 1, PageSize: 10, List: []dto.ArticleListItemResp{}},
	}
	r := newArticleRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles?page=1&page_size=10&recommend=true", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, stub.listReq.Page)
	assert.Equal(t, 10, stub.listReq.PageSize)
	require.NotNil(t, stub.listReq.Recommend)
	assert.True(t, *stub.listReq.Recommend)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestArticleHandler_ListPublic_PassesOptionalViewerID(t *testing.T) {
	stub := &stubArticleService{
		listResp: &dto.ArticlePageResp{Page: 1, PageSize: 10, List: []dto.ArticleListItemResp{}},
	}
	r := newArticleRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/authed/articles", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, stub.listViewer)
	assert.Equal(t, uint(9), *stub.listViewer)
}

func TestArticleHandler_GetPublicDetail_NotFound(t *testing.T) {
	r := newArticleRouter(&stubArticleService{detailErr: articleservice.ErrArticleNotFound})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles/404", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestArticleHandler_Save_UsesClaimsUserID(t *testing.T) {
	stub := &stubArticleService{
		saveResp: &dto.ArticleDetailResp{ArticleListItemResp: dto.ArticleListItemResp{ID: 9, Title: "A"}},
	}
	r := newArticleRouter(stub)
	body, _ := json.Marshal(dto.ArticleSaveReq{
		Title:         "A",
		Content:       "body",
		Status:        1,
		CommentStatus: 1,
		CategoryIDs:   []uint{1},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.saveUserID)
	assert.Equal(t, "A", stub.saveReq.Title)
}

func TestArticleHandler_Save_BadRequest(t *testing.T) {
	r := newArticleRouter(&stubArticleService{saveErr: articleservice.ErrArticleCategoryRequired})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/articles", bytes.NewReader([]byte(`{"title":"A"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestArticleHandler_ListPublic_ServerError(t *testing.T) {
	r := newArticleRouter(&stubArticleService{listErr: errors.New("db down")})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestArticleHandler_ListPublic_InvalidPageSizeReturnsReadableMessage(t *testing.T) {
	r := newArticleRouter(&stubArticleService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles?page_size=100", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "每页数量不能大于 50", resp.Message)
}

func TestArticleHandler_GetPublicDetail_InvalidIDReturnsReadableMessage(t *testing.T) {
	r := newArticleRouter(&stubArticleService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles/abc", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "文章 ID 必须是大于 0 的整数", resp.Message)
}

func TestArticleHandler_ToggleLike_ReturnsLatestLikeState(t *testing.T) {
	stub := &stubArticleService{
		likeResp: &dto.ArticleLikeResp{IsLiked: true, LikeCount: 12},
	}
	r := newArticleRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/articles/12/like", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}
