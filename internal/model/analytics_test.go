package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/model"
)

func TestAnalyticsTableNames(t *testing.T) {
	assert.Equal(t, "analytics_events", model.AnalyticsEvent{}.TableName())
	assert.Equal(t, "analytics_sessions", model.AnalyticsSession{}.TableName())
	assert.Equal(t, "analytics_daily", model.AnalyticsDaily{}.TableName())
	assert.Equal(t, "analytics_daily_dim", model.AnalyticsDailyDim{}.TableName())
	assert.Equal(t, "analytics_page_daily", model.AnalyticsPageDaily{}.TableName())
}
