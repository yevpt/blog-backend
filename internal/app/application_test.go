package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/app"
)

func TestBuildRejectsIncompleteDependencies(t *testing.T) {
	application, err := app.Build(context.Background(), app.Dependencies{})

	require.ErrorContains(t, err, "应用基础设施依赖不完整")
	require.Nil(t, application)
}
