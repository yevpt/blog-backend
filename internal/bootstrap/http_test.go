package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vpt/blog-backend/pkg/config"
)

func TestNewHTTPServerAppliesProductionSafeTimeouts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	server := newHTTPServer(engine, &config.Config{
		Server: config.ServerConfig{Port: 8080},
	})

	assert.Equal(t, ":8080", server.Addr)
	assert.Equal(t, 5*time.Second, server.ReadHeaderTimeout)
	assert.Equal(t, 30*time.Second, server.ReadTimeout)
	assert.Equal(t, 60*time.Second, server.WriteTimeout)
	assert.Equal(t, 120*time.Second, server.IdleTimeout)
	assert.Equal(t, 1<<20, server.MaxHeaderBytes)
}

func TestRunHTTPServerStopsWhenContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := newHTTPServer(gin.New(), &config.Config{
		Server: config.ServerConfig{Port: 0},
	})

	require.NoError(t, runHTTPServer(ctx, server, zap.NewNop()))
}
