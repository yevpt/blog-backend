package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	articlehandler "github.com/vpt/blog-backend/internal/handler/article"
	articleservice "github.com/vpt/blog-backend/internal/service/article"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubArticleServiceForRouter struct {
	listViewerID *uint
}

func (s *stubArticleServiceForRouter) ListIDs() (*dto.ArticleIDsResp, error) {
	return &dto.ArticleIDsResp{}, nil
}

func (s *stubArticleServiceForRouter) ListPublic(req dto.ArticleListReq, viewerID *uint) (*dto.ArticlePageResp, error) {
	s.listViewerID = viewerID
	return &dto.ArticlePageResp{Page: 1, PageSize: 10, List: []dto.ArticleListItemResp{}}, nil
}

func (s *stubArticleServiceForRouter) GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	return &dto.ArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) GetAdminDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	return &dto.ArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error) {
	return &dto.ArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) Delete(id uint) (*dto.ArticleDetailResp, error) {
	return &dto.ArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) View(id uint, visitorID string) (*dto.ArticleViewResp, error) {
	return &dto.ArticleViewResp{ID: id, ViewCount: 1}, nil
}

func (s *stubArticleServiceForRouter) IsLiked(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	return &dto.ArticleLikeResp{IsLiked: true, LikeCount: 1}, nil
}

func (s *stubArticleServiceForRouter) ToggleLike(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	return &dto.ArticleLikeResp{IsLiked: true, LikeCount: 1}, nil
}

var _ articleservice.ArticleService = (*stubArticleServiceForRouter)(nil)

func TestRegisterPublicRoutes_ArticlesListAllowsOptionalAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	jwtManager := jwt.NewManager("test-secret", 2, 24)
	stubSvc := &stubArticleServiceForRouter{}

	registerPublicRoutes(r, routeHandlers{
		article: articlehandler.NewArticleHandler(stubSvc),
	}, jwtManager, nil)

	token, err := jwtManager.GenerateAccess(9)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/articles?page=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, stubSvc.listViewerID)
	assert.Equal(t, uint(9), *stubSvc.listViewerID)

	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}
