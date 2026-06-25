package analytics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	hdl "github.com/vpt/blog-backend/internal/handler/analytics"
	"github.com/vpt/blog-backend/pkg/response"
)

// fakeQuery 记录最后一次调用入参，便于断言默认值与限幅逻辑。
type fakeQuery struct {
	lastFrom, lastTo, lastMetric, lastSegment string
	lastDimension                             string
	lastLimit                                 int
}

func (f *fakeQuery) Overview(context.Context) (dto.Overview, error) {
	return dto.Overview{TodayPV: 10, Online: 2}, nil
}

func (f *fakeQuery) Trend(_ context.Context, from, to, metric, segment string) ([]dto.TrendPoint, error) {
	f.lastFrom, f.lastTo, f.lastMetric, f.lastSegment = from, to, metric, segment
	return []dto.TrendPoint{{Date: "2026-06-24", Value: 10}}, nil
}

func (f *fakeQuery) TopPages(_ context.Context, from, to string, limit int) ([]dto.PageStat, error) {
	f.lastFrom, f.lastTo, f.lastLimit = from, to, limit
	return []dto.PageStat{{Path: "/", PV: 5}}, nil
}

func (f *fakeQuery) Dimensions(_ context.Context, dimension, from, to string) ([]dto.DimensionPoint, error) {
	f.lastDimension, f.lastFrom, f.lastTo = dimension, from, to
	return []dto.DimensionPoint{{Date: "2026-06-01", DimValue: "desktop", PV: 10, UV: 5}}, nil
}

type fakeBackfill struct {
	lastFrom, lastTo string
}

func (f *fakeBackfill) Backfill(_ context.Context, from, to string) (int, error) {
	f.lastFrom, f.lastTo = from, to
	return 3, nil
}

func newRouter(h *hdl.AdminHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/analytics/overview", h.Overview)
	r.GET("/admin/analytics/trend", h.Trend)
	r.GET("/admin/analytics/pages", h.Pages)
	r.GET("/admin/analytics/dimensions", h.Dimensions)
	r.POST("/admin/analytics/backfill", h.Backfill)
	return r
}

func doGET(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func doPOST(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestAdminOverview(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/overview")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "today_pv")
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
}

func TestAdminTrend_HappyPathDefaults(t *testing.T) {
	fq := &fakeQuery{}
	r := newRouter(hdl.NewAdminHandler(fq, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/trend")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	// 默认 metric=pv、segment=all，from/to 为近 7 天的 YYYY-MM-DD。
	assert.Equal(t, "pv", fq.lastMetric)
	assert.Equal(t, "all", fq.lastSegment)
	assert.Len(t, fq.lastFrom, len("2006-01-02"))
	assert.Len(t, fq.lastTo, len("2006-01-02"))
}

func TestAdminTrend_ExplicitParams(t *testing.T) {
	fq := &fakeQuery{}
	r := newRouter(hdl.NewAdminHandler(fq, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/trend?from=2026-06-01&to=2026-06-10&metric=uv&segment=registered")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "2026-06-01", fq.lastFrom)
	assert.Equal(t, "2026-06-10", fq.lastTo)
	assert.Equal(t, "uv", fq.lastMetric)
	assert.Equal(t, "registered", fq.lastSegment)
}

func TestAdminTrend_InvalidMetric(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/trend?metric=bogus")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminTrend_InvalidSegment(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/trend?segment=bogus")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminTrend_RangeTooLarge(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/trend?from=2024-01-01&to=2026-06-01")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminTrend_InvalidDateFormat(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/trend?from=2026/06/01")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminDimensions_HappyPath(t *testing.T) {
	fq := &fakeQuery{}
	r := newRouter(hdl.NewAdminHandler(fq, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/dimensions?dimension=device")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	assert.Equal(t, "device", fq.lastDimension)
}

func TestAdminDimensions_InvalidDimension(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/dimensions?dimension=bogus")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminDimensions_MissingDimension(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/dimensions")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminPages_LimitCap(t *testing.T) {
	fq := &fakeQuery{}
	r := newRouter(hdl.NewAdminHandler(fq, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/pages?limit=500")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	assert.Equal(t, 100, fq.lastLimit) // 上限 100
}

func TestAdminPages_LimitDefault(t *testing.T) {
	fq := &fakeQuery{}
	r := newRouter(hdl.NewAdminHandler(fq, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/pages")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 20, fq.lastLimit) // 默认 20
}

func TestAdminPages_RangeTooLarge(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doGET(r, "/admin/analytics/pages?"+url.Values{
		"from": {"2024-01-01"}, "to": {"2026-06-01"},
	}.Encode())

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminBackfill_HappyPath(t *testing.T) {
	fb := &fakeBackfill{}
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, fb))

	w := doPOST(r, "/admin/analytics/backfill?from=2026-06-01&to=2026-06-03")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	assert.Equal(t, "2026-06-01", fb.lastFrom)
	assert.Equal(t, "2026-06-03", fb.lastTo)
	assert.Contains(t, w.Body.String(), "days")
}

func TestAdminBackfill_MissingParams(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doPOST(r, "/admin/analytics/backfill")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminBackfill_RangeTooLarge(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))

	w := doPOST(r, "/admin/analytics/backfill?from=2026-01-01&to=2026-06-01")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}
