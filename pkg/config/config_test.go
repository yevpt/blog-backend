package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/config"
)

// TestLoad_ReadsGarageAndCDNConfig 验证配置加载能解析 Garage 和 CDN 配置。
func TestLoad_ReadsGarageAndCDNConfig(t *testing.T) {
	// 记录当前工作目录，测试结束后恢复，避免影响其他测试。
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(cwd))
	})

	// 在临时目录创建最小配置文件，避免读取开发机本地 config.local.yaml。
	configDir := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
garage:
  endpoint: "https://garage.example.com"
  bucket: "blog"
  region: "garage"
  accessKeyID: "yaml-access-key"
  secretAccessKey: "yaml-secret-key"
  cdn: true
cdn:
  host: "https://blog-oss.example.com"
  secret: "cdn-secret"
  signQueryName: "a"
  timestampQueryName: "b"
`), 0o644))

	// 清空环境配置并切换工作目录，让 Load 只读取临时配置。
	t.Setenv("APP_ENV", "")
	require.NoError(t, os.Chdir(filepath.Dir(configDir)))

	// 加载配置。
	cfg, err := config.Load()
	require.NoError(t, err)

	// 校验 Garage 和 CDN 字段被正确映射到结构体。
	assert.Equal(t, "blog", cfg.Garage.Bucket)
	assert.Equal(t, "garage", cfg.Garage.Region)
	assert.Equal(t, "yaml-access-key", cfg.Garage.AccessKeyID)
	assert.Equal(t, "yaml-secret-key", cfg.Garage.SecretAccessKey)
	assert.Equal(t, "a", cfg.CDN.SignQueryName)
	assert.Equal(t, "b", cfg.CDN.TimestampQueryName)
}
