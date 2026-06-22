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

// TestLoad_ReadsEmailWorkerConfig 验证邮件 worker 与安全限额配置能解析到结构体。
func TestLoad_ReadsEmailWorkerConfig(t *testing.T) {
	// 记录当前工作目录，测试结束后恢复，避免影响其他测试。
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(cwd))
	})

	// 在临时目录写入含完整 email 配置的最小配置文件。
	configDir := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
email:
  provider: aliyun_enterprise
  host: smtp.qiye.aliyun.com
  port: 465
  from: noreply@example.com
  password: "yaml-pass"
  provider_daily_hard_limit: 2000
  site_daily_safe_limit: 300
  max_per_minute: 5
  max_per_hour: 80
  send_interval_seconds: 12
  worker_enabled: true
  planner_enabled: true
  worker_batch_size: 20
  lease_seconds: 300
`), 0o644))

	// 清空环境配置并切换工作目录，让 Load 只读取临时配置。
	t.Setenv("APP_ENV", "")
	require.NoError(t, os.Chdir(filepath.Dir(configDir)))

	// 加载配置。
	cfg, err := config.Load()
	require.NoError(t, err)

	// 校验所有新增的 worker 与安全限额字段被正确映射。
	assert.Equal(t, "aliyun_enterprise", cfg.Email.Provider)
	assert.Equal(t, 2000, cfg.Email.ProviderDailyHardLimit)
	assert.Equal(t, 300, cfg.Email.SiteDailySafeLimit)
	assert.Equal(t, 5, cfg.Email.MaxPerMinute)
	assert.Equal(t, 80, cfg.Email.MaxPerHour)
	assert.Equal(t, 12, cfg.Email.SendIntervalSeconds)
	assert.True(t, cfg.Email.WorkerEnabled)
	assert.True(t, cfg.Email.PlannerEnabled)
	assert.Equal(t, 20, cfg.Email.WorkerBatchSize)
	assert.Equal(t, 300, cfg.Email.LeaseSeconds)
}

// TestLoad_ReadsEmailWorkerEnvOverride 验证 worker 开关可通过环境变量覆盖。
func TestLoad_ReadsEmailWorkerEnvOverride(t *testing.T) {
	// 记录当前工作目录，测试结束后恢复，避免影响其他测试。
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(cwd))
	})

	// 基础配置默认开启 worker，准备用环境变量关闭。
	configDir := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
email:
  worker_enabled: true
  planner_enabled: true
`), 0o644))

	// 通过环境变量关闭 worker 与 planner，模拟生产差异化部署。
	t.Setenv("APP_ENV", "")
	t.Setenv("BLOG_EMAIL_WORKER_ENABLED", "false")
	t.Setenv("BLOG_EMAIL_PLANNER_ENABLED", "false")
	require.NoError(t, os.Chdir(filepath.Dir(configDir)))

	// 加载配置。
	cfg, err := config.Load()
	require.NoError(t, err)

	// 校验环境变量覆盖了 YAML 中的默认开关。
	assert.False(t, cfg.Email.WorkerEnabled)
	assert.False(t, cfg.Email.PlannerEnabled)
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
