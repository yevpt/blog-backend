package imagecdn_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/imagecdn"
)

func TestParseTransformParams_WithWidth(t *testing.T) {
	cfg := config.ImageConfig{DefaultQuality: 75, MaxWidth: 3840}
	w, q, transform := imagecdn.ParseTransformParams(url.Values{"w": {"640"}, "q": {"60"}}, cfg)
	assert.True(t, transform)
	assert.Equal(t, 640, w)
	assert.Equal(t, 60, q)
}

func TestParseTransformParams_WithoutWidth(t *testing.T) {
	cfg := config.ImageConfig{DefaultQuality: 75, MaxWidth: 3840}
	_, _, transform := imagecdn.ParseTransformParams(url.Values{}, cfg)
	assert.False(t, transform)
}

func TestParseTransformParams_ClampWidth(t *testing.T) {
	cfg := config.ImageConfig{DefaultQuality: 75, MaxWidth: 1000}
	w, _, transform := imagecdn.ParseTransformParams(url.Values{"w": {"2000"}}, cfg)
	assert.True(t, transform)
	assert.Equal(t, 1000, w)
}
