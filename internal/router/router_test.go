package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	articlehandler "github.com/vpt/blog-backend/internal/handler/article"
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	articleservice "github.com/vpt/blog-backend/internal/service/article"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/config"
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

func (s *stubArticleServiceForRouter) ListAdmin(req dto.AdminArticleListReq) (*dto.AdminArticlePageResp, error) {
	return &dto.AdminArticlePageResp{Page: 1, PageSize: 10, List: []dto.AdminArticleListItemResp{}}, nil
}

func (s *stubArticleServiceForRouter) GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	return &dto.ArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) GetAdminDetail(id uint, viewerID *uint) (*dto.AdminArticleDetailResp, error) {
	return &dto.AdminArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error) {
	return &dto.ArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) Delete(id uint) (*dto.ArticleDetailResp, error) {
	return &dto.ArticleDetailResp{}, nil
}

func (s *stubArticleServiceForRouter) PermanentDelete(id uint, operatorID uint) (*dto.ArticleDeleteResp, error) {
	return &dto.ArticleDeleteResp{ID: id}, nil
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

func (s *stubArticleServiceForRouter) ListRecommendedAdmin() (*dto.AdminRecommendListResp, error) {
	return &dto.AdminRecommendListResp{}, nil
}

func (s *stubArticleServiceForRouter) ReorderRecommendedAdmin(req dto.AdminRecommendOrderReq) error {
	return nil
}

var _ articleservice.ArticleService = (*stubArticleServiceForRouter)(nil)

type moderationReviewStub struct{}

func (moderationReviewStub) List(context.Context, moderationservice.ListReviewCommand) (moderationservice.ReviewPage, error) {
	return moderationservice.ReviewPage{}, nil
}

func (moderationReviewStub) Get(context.Context, uint64) (moderationservice.ReviewItem, error) {
	return moderationservice.ReviewItem{}, nil
}

func (moderationReviewStub) Approve(context.Context, moderationservice.ReviewCommand) (moderationservice.ReviewItem, error) {
	return moderationservice.ReviewItem{}, nil
}

func (moderationReviewStub) Correct(context.Context, moderationservice.CorrectCommand) (moderationservice.ReviewItem, error) {
	return moderationservice.ReviewItem{}, nil
}

func (moderationReviewStub) Reject(context.Context, moderationservice.ReviewCommand) (moderationservice.ReviewItem, error) {
	return moderationservice.ReviewItem{}, nil
}

func TestRegisterAdminRoutesSkipsModerationWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	jwtManager := jwt.NewManager("test-secret", 2, 24)

	registerAdminRoutes(r, routeHandlers{}, jwtManager, nil)

	for _, route := range r.Routes() {
		assert.NotContains(t, route.Path, "/moderation/")
	}
}

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

func TestNewOAuthManager_RegistersEnabledSocialProviders(t *testing.T) {
	cfg := &config.Config{
		OAuth: config.OAuthConfig{
			Providers: map[string]config.OAuthProviderConfig{
				"github": {Enabled: true},
				"gitee":  {Enabled: true},
				"qq":     {Enabled: true},
				"weibo":  {Enabled: true},
				"baidu":  {Enabled: true},
				"google": {Enabled: false},
			},
		},
	}

	manager := newOAuthManager(nil, cfg)

	assert.Equal(t, []string{"baidu", "gitee", "github", "qq", "weibo"}, manager.Sources())
}

func TestRegisterAuthedRoutes_RegistersTempUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	jwtManager := jwt.NewManager("test-secret", 2, 24)

	registerAuthedRoutes(r, routeHandlers{}, jwtManager, nil)

	paths := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		if route.Method == http.MethodPost {
			paths = append(paths, route.Path)
		}
	}
	assert.True(t, slices.Contains(paths, "/uploads/temp"))
}

func TestRegisterAuthedRoutesRegistersModerationEditRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwtManager := jwt.NewManager("test-secret", 2, 24)

	registerAuthedRoutes(router, routeHandlers{}, jwtManager, nil)

	want := map[string]bool{
		"/articles/comments/:id": false, "/articles/comment-replies/:id": false,
		"/moments/comments/:id": false, "/moments/comment-replies/:id": false,
		"/guestbook/:id": false, "/guestbook/comment-replies/:id": false,
	}
	for _, route := range router.Routes() {
		if route.Method == http.MethodPatch {
			if _, ok := want[route.Path]; ok {
				want[route.Path] = true
			}
		}
	}
	for path, registered := range want {
		assert.True(t, registered, path)
	}
}

func TestCORSAllowsModerationIdempotencyHeader(t *testing.T) {
	assert.Contains(t, newCORSConfig().AllowHeaders, "Idempotency-Key")
}

func TestRegisterAdminRoutes_RegistersFriendLinkMutationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	jwtManager := jwt.NewManager("test-secret", 2, 24)

	registerAdminRoutes(r, routeHandlers{}, jwtManager, nil)

	paths := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		if route.Method == http.MethodPost {
			paths = append(paths, route.Path)
		}
	}
	assert.True(t, slices.Contains(paths, "/admin/friend-links"))
}

func TestRegisterAdminRoutesRegistersModerationReviewRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	jwtManager := jwt.NewManager("test-secret", 2, 24)

	registerAdminRoutes(r, routeHandlers{
		moderationAdmin: moderationhandler.NewAdminHandler(&moderationReviewStub{}),
	}, jwtManager, nil)

	want := map[string]string{
		"GET /admin/moderation/items":              "",
		"GET /admin/moderation/items/:id":          "",
		"POST /admin/moderation/items/:id/approve": "",
		"POST /admin/moderation/items/:id/correct": "",
		"POST /admin/moderation/items/:id/reject":  "",
	}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = route.Handler
		}
	}
	for route, handler := range want {
		assert.NotEmpty(t, handler, route)
	}
}
