package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"go.uber.org/zap"
)

func TestGeoResolverNoopWhenMissing(t *testing.T) {
	r := svc.NewGeoResolver("", zap.NewNop())
	country, region := r.Resolve("1.2.3.4")
	assert.Equal(t, "", country)
	assert.Equal(t, "", region)

	r2 := svc.NewGeoResolver("/nonexistent/path.xdb", zap.NewNop())
	c2, rg2 := r2.Resolve("1.2.3.4")
	assert.Equal(t, "", c2)
	assert.Equal(t, "", rg2)
}
