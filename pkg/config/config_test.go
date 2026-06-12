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

// TestLoad_ReadsOAuthProvidersConfig 验证配置加载能解析 OAuth 平台配置。
func TestLoad_ReadsOAuthProvidersConfig(t *testing.T) {
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
oauth:
  state_ttl_minutes: 10
  providers:
    github:
      enabled: true
      client_id: "github-client-id"
      client_secret: "github-client-secret"
      redirect_uri: "http://localhost:8080/oauth/github/callback"
      scopes:
        - "read:user"
        - "user:email"
      auth_url: "https://github.com/login/oauth/authorize"
      token_url: "https://github.com/login/oauth/access_token"
      user_url: "https://api.github.com/user"
      openid_url: "https://graph.qq.com/oauth2.0/me"
`), 0o644))

	// 清空环境配置并切换工作目录，让 Load 只读取临时配置。
	t.Setenv("APP_ENV", "")
	require.NoError(t, os.Chdir(filepath.Dir(configDir)))

	// 加载配置。
	cfg, err := config.Load()
	require.NoError(t, err)

	// 校验 OAuth provider map 被正确映射到结构体。
	github := cfg.OAuth.Providers["github"]
	assert.Equal(t, 10, cfg.OAuth.StateTTLMinutes)
	assert.True(t, github.Enabled)
	assert.Equal(t, "github-client-id", github.ClientID)
	assert.Equal(t, "github-client-secret", github.ClientSecret)
	assert.Equal(t, "http://localhost:8080/oauth/github/callback", github.RedirectURI)
	assert.Equal(t, []string{"read:user", "user:email"}, github.Scopes)
	assert.Equal(t, "https://github.com/login/oauth/authorize", github.AuthURL)
	assert.Equal(t, "https://github.com/login/oauth/access_token", github.TokenURL)
	assert.Equal(t, "https://api.github.com/user", github.UserURL)
	assert.Equal(t, "https://graph.qq.com/oauth2.0/me", github.OpenIDURL)
}

// TestLoad_ReadsEnvOnlyRuntimeConfig 验证只通过环境变量注入的运行时配置能写入结构体。
func TestLoad_ReadsEnvOnlyRuntimeConfig(t *testing.T) {
	// 记录当前工作目录，测试结束后恢复，避免影响其他测试。
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(cwd))
	})

	// 创建接近生产镜像内的基础配置：敏感连接信息不写入 YAML，只通过环境变量注入。
	configDir := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
server:
  port: 8080
db:
  max_open_conns: 10
  max_idle_conns: 5
  max_lifetime_minutes: 30
redis:
  db: 0
garage:
  cdn: false
`), 0o644))

	// 通过环境变量模拟 Docker Compose 注入的生产连接信息。
	t.Setenv("APP_ENV", "")
	t.Setenv("BLOG_DB_HOST", "192.168.2.3")
	t.Setenv("BLOG_DB_PORT", "9003")
	t.Setenv("BLOG_DB_NAME", "blog_dev")
	t.Setenv("BLOG_DB_USER", "blog_dev")
	t.Setenv("BLOG_DB_PASSWORD", "db-secret")
	t.Setenv("BLOG_REDIS_ADDR", "192.168.2.3:9004")
	t.Setenv("BLOG_REDIS_PASSWORD", "redis-secret")
	t.Setenv("BLOG_GARAGE_ENDPOINT", "http://garage.example.com")
	t.Setenv("BLOG_GARAGE_ACCESSKEYID", "garage-access")
	t.Setenv("BLOG_GARAGE_SECRETACCESSKEY", "garage-secret")
	require.NoError(t, os.Chdir(filepath.Dir(configDir)))

	// 加载配置。
	cfg, err := config.Load()
	require.NoError(t, err)

	// 校验环境变量中的连接信息没有丢失，避免生成 tcp :0 这类空 DSN。
	assert.Equal(t, "192.168.2.3", cfg.DB.Host)
	assert.Equal(t, 9003, cfg.DB.Port)
	assert.Equal(t, "blog_dev", cfg.DB.Name)
	assert.Equal(t, "blog_dev", cfg.DB.User)
	assert.Equal(t, "db-secret", cfg.DB.Password)
	assert.Equal(t, "192.168.2.3:9004", cfg.Redis.Addr)
	assert.Equal(t, "redis-secret", cfg.Redis.Password)
	assert.Equal(t, "http://garage.example.com", cfg.Garage.Endpoint)
	assert.Equal(t, "garage-access", cfg.Garage.AccessKeyID)
	assert.Equal(t, "garage-secret", cfg.Garage.SecretAccessKey)
}
