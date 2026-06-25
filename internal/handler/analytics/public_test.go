package analytics_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	hdl "github.com/vpt/blog-backend/internal/handler/analytics"
	"github.com/vpt/blog-backend/pkg/response"
)

type fakePublic struct{ lastLimit int }

func (f *fakePublic) Summary(context.Context) (dto.PublicSummary, error) {
	return dto.PublicSummary{TodayPV: 5, Online: 2}, nil
}
func (f *fakePublic) Popular(_ context.Context, limit int) ([]dto.PublicPageStat, error) {
	f.lastLimit = limit
	return []dto.PublicPageStat{{Path: "/a", PV: 9}}, nil
}

func newPublicRouter(h *hdl.PublicHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/analytics/public/summary", h.Summary)
	r.GET("/analytics/public/popular", h.Popular)
	return r
}

func TestPublicSummaryHandler(t *testing.T) {
	r := newPublicRouter(hdl.NewPublicHandler(&fakePublic{}))
	w := doGET(r, "/analytics/public/summary")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	assert.Contains(t, w.Body.String(), "today_pv")
}

func TestPublicPopular_LimitCap(t *testing.T) {
	fp := &fakePublic{}
	r := newPublicRouter(hdl.NewPublicHandler(fp))
	w := doGET(r, "/analytics/public/popular?limit=500")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 20, fp.lastLimit)
}

func TestPublicPopular_InvalidLimit(t *testing.T) {
	r := newPublicRouter(hdl.NewPublicHandler(&fakePublic{}))
	w := doGET(r, "/analytics/public/popular?limit=abc")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}
