package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"go.uber.org/zap"
)

func TestGeoResolverNoopWhenMissing(t *testing.T) {
	r := svc.NewGeoResolver("", "", zap.NewNop())
	geo := r.Resolve("1.2.3.4")
	assert.Equal(t, svc.GeoInfo{}, geo)

	r2 := svc.NewGeoResolver("/nonexistent/v4.xdb", "/nonexistent/v6.xdb", zap.NewNop())
	geo2 := r2.Resolve("1.2.3.4")
	assert.Equal(t, svc.GeoInfo{}, geo2)
}

func TestGeoInfoFromRegion(t *testing.T) {
	got := svc.GeoInfoFromRegion("中国|浙江省|杭州市|电信|CN")

	assert.Equal(t, "中国", got.Country)
	assert.Equal(t, "浙江省", got.Region)
	assert.Equal(t, "杭州市", got.City)
	assert.Equal(t, "电信", got.ISP)
	assert.Equal(t, "CN", got.CountryCode)
}
