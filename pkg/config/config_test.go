package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/config"
)

// TestLoad_ReadsModerationConfig 验证审核配置能完整解析为强类型字段。
func TestLoad_ReadsModerationConfig(t *testing.T) {
	// 记录当前工作目录，测试结束后恢复，避免影响其他测试。
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(cwd))
	})

	// 写入覆盖全部审核配置段的配置文件，确保嵌套字段不会被静默忽略。
	configDir := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
moderation:
  enabled: true
  mode: enforce
  policy:
    new: {clean_low: post_review, unapproved_image: pre_review, external_link_or_ad: pre_review, medium: pre_review, high: block}
    normal: {clean_low: post_review, unapproved_image: post_review, external_link_or_ad: post_review, medium: pre_review, high: block}
    trusted: {clean_low: auto_approve, unapproved_image: post_review, external_link_or_ad: pre_review, medium: pre_review, high: block}
    restricted: {clean_low: pre_review, unapproved_image: pre_review, external_link_or_ad: pre_review, medium: pre_review, high: block}
  rules:
    max_pattern_chars: 500
    max_enabled_regex_rules: 200
    require_non_empty_in_enforce: true
  content:
    moment_max_chars: 800
    comment_max_chars: 2000
    guestbook_max_chars: 2000
    reply_max_chars: 2000
    max_images_per_content: 9
    max_links_per_content: 10
  image:
    max_upload_bytes: 1048576
    max_gif_bytes: 307200
    max_stored_bytes: 512000
    max_pixels: 12000000
    processing_concurrency: 2
    preview_max_edge: 48
    static_placeholder_key: system/moderation/image-review.jpg
    gif_placeholder_key: system/moderation/gif-review.jpg
    approval_retention_days: 180
    temp_retention: 24h
    orphan_min_age: 24h
    cleanup_interval: 24h
    cleanup_batch_size: 500
  review:
    queue_default_page_size: 20
    queue_max_page_size: 100
    reason_max_chars: 1000
  governance:
    new_to_normal: {min_age_days: 7, clean_approvals: 3}
    normal_to_trusted: {min_age_days: 30, clean_approvals: 20}
    restricted_score_threshold: 6
    restricted_duration: 168h
    clean_approval_score_decay: 1
    violation_weights: {corrected: 1, rejected: 3, high_risk_blocked: 5}
  rate_limit:
    publish_per_minute: 10
    edit_per_minute: 10
    temp_upload_per_minute: 10
  control:
    default_registration_mode: open
    default_publishing_mode: open
    user_hide_batch_size: 200
    user_hide_max_items_per_request: 1000
  audit:
    attempt_retention_days: 180
    action_log_retention_days: 365
    obsolete_revision_retention_days: 365
    cleanup_interval: 24h
    cleanup_batch_size: 500
  migration:
    batch_size: 200
  notices:
    approved: 发布成功。
    low_submitted: 发布成功，内容会被审核。
    review_required: 内容已提交，等待人工审核。
    high_rejected: 内容存在较高风险，未能发布，请修改后重试。
`), 0o644))

	t.Setenv("APP_ENV", "")
	require.NoError(t, os.Chdir(filepath.Dir(configDir)))

	// 加载并校验 Core 及后续阶段使用的代表字段。
	cfg, err := config.Load()
	require.NoError(t, err)
	require.NoError(t, cfg.Moderation.Validate("test"))
	assert.True(t, cfg.Moderation.Enabled)
	assert.Equal(t, config.ModerationModeEnforce, cfg.Moderation.Mode)
	assert.Equal(t, config.ModerationActionAutoApprove, cfg.Moderation.Policy.Trusted.CleanLow)
	assert.Equal(t, 500, cfg.Moderation.Rules.MaxPatternChars)
	assert.Equal(t, 800, cfg.Moderation.Content.MomentMaxChars)
	assert.Equal(t, int64(1_048_576), cfg.Moderation.Image.MaxUploadBytes)
	assert.Equal(t, 20, cfg.Moderation.Review.QueueDefaultPageSize)
	assert.Equal(t, 168*time.Hour, cfg.Moderation.Governance.RestrictedDuration)
	assert.Equal(t, 10, cfg.Moderation.RateLimit.PublishPerMinute)
	assert.Equal(t, 365, cfg.Moderation.Audit.ActionLogRetentionDays)
	assert.Equal(t, 200, cfg.Moderation.Migration.BatchSize)
	assert.Equal(t, "发布成功，内容会被审核。", cfg.Moderation.Notices.LowSubmitted)
}

// TestLoad_ValidatesModerationForActualEnvironment 验证 Load 使用 APP_ENV 对应的最终配置执行审核校验。
func TestLoad_ValidatesModerationForActualEnvironment(t *testing.T) {
	tests := []struct {
		name         string
		prodOverride string
		wantErr      string
	}{
		{
			name:         "valid production config loads",
			prodOverride: projectConfigFile(t, "config.prod.yaml"),
		},
		{
			name: "invalid production mode is rejected after overlay",
			prodOverride: `
moderation:
  enabled: true
  mode: observe
`,
			wantErr: "校验审核配置失败: moderation.mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 保存工作目录并构造不受本地私有配置影响的完整配置目录。
			cwd, err := os.Getwd()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, os.Chdir(cwd))
			})

			configDir := filepath.Join(t.TempDir(), "config")
			require.NoError(t, os.MkdirAll(configDir, 0o755))
			require.NoError(t, os.WriteFile(
				filepath.Join(configDir, "config.yaml"),
				[]byte(projectConfigFile(t, "config.yaml")),
				0o644,
			))
			require.NoError(t, os.WriteFile(
				filepath.Join(configDir, "config.prod.yaml"),
				[]byte(tt.prodOverride),
				0o644,
			))

			t.Setenv("APP_ENV", "prod")
			require.NoError(t, os.Chdir(filepath.Dir(configDir)))

			cfg, err := config.Load()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, cfg)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg)
			assert.Equal(t, config.ModerationModeEnforce, cfg.Moderation.Mode)
		})
	}
}

func projectConfigFile(t *testing.T, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("..", "..", "config", name))
	require.NoError(t, err)
	return string(content)
}

// TestValidateModeration 验证审核配置拒绝不安全策略和无效边界。
func TestValidateModeration(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.ModerationConfig)
		env     string
		wantErr string
	}{
		{name: "valid defaults", env: "production"},
		{name: "production disabled", env: "production", mutate: func(c *config.ModerationConfig) {
			c.Enabled = false
		}},
		{name: "production requires enforce", env: "production", mutate: func(c *config.ModerationConfig) {
			c.Mode = config.ModerationModeObserve
		}, wantErr: "moderation.mode"},
		{name: "empty notices", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Notices.ReviewRequired = ""
		}, wantErr: "moderation.notices.review_required"},
		{name: "empty approved notice", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Notices.Approved = ""
		}, wantErr: "moderation.notices.approved"},
		{name: "invalid action", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Policy.Normal.Medium = "publish"
		}, wantErr: "moderation.policy.normal.medium"},
		{name: "high is always blocked", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Policy.Normal.High = config.ModerationActionPreReview
		}, wantErr: "moderation.policy.normal.high"},
		{name: "restricted cannot post review", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Policy.Restricted.CleanLow = config.ModerationActionPostReview
		}, wantErr: "moderation.policy.restricted.clean_low"},
		{name: "restricted cannot auto approve", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Policy.Restricted.Medium = config.ModerationActionAutoApprove
		}, wantErr: "moderation.policy.restricted.medium"},
		{name: "unapproved image cannot auto approve", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Policy.Trusted.UnapprovedImage = config.ModerationActionAutoApprove
		}, wantErr: "moderation.policy.trusted.unapproved_image"},
		{name: "rules bounds are positive", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Rules.MaxPatternChars = 0
		}, wantErr: "moderation.rules.max_pattern_chars"},
		{name: "content bounds are positive", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Content.ReplyMaxChars = -1
		}, wantErr: "moderation.content.reply_max_chars"},
		{name: "rate limits are positive", env: "test", mutate: func(c *config.ModerationConfig) {
			c.RateLimit.EditPerMinute = 0
		}, wantErr: "moderation.rate_limit.edit_per_minute"},
		{name: "image bounds are positive", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Image.MaxPixels = 0
		}, wantErr: "moderation.image.max_pixels"},
		{name: "governance bounds are positive", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Governance.NewToNormal.CleanApprovals = 0
		}, wantErr: "moderation.governance.new_to_normal.clean_approvals"},
		{name: "control bounds are positive", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Control.UserHideBatchSize = 0
		}, wantErr: "moderation.control.user_hide_batch_size"},
		{name: "audit bounds are positive", env: "test", mutate: func(c *config.ModerationConfig) {
			c.Audit.CleanupBatchSize = 0
		}, wantErr: "moderation.audit.cleanup_batch_size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validModerationConfig()
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := cfg.Validate(tt.env)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func validModerationConfig() config.ModerationConfig {
	return config.ModerationConfig{
		Enabled: true,
		Mode:    config.ModerationModeEnforce,
		Policy: config.ModerationPolicyConfig{
			New:        config.ModerationPolicyActionsConfig{CleanLow: "post_review", UnapprovedImage: "pre_review", ExternalLinkOrAd: "pre_review", Medium: "pre_review", High: "block"},
			Normal:     config.ModerationPolicyActionsConfig{CleanLow: "post_review", UnapprovedImage: "post_review", ExternalLinkOrAd: "post_review", Medium: "pre_review", High: "block"},
			Trusted:    config.ModerationPolicyActionsConfig{CleanLow: "auto_approve", UnapprovedImage: "post_review", ExternalLinkOrAd: "pre_review", Medium: "pre_review", High: "block"},
			Restricted: config.ModerationPolicyActionsConfig{CleanLow: "pre_review", UnapprovedImage: "pre_review", ExternalLinkOrAd: "pre_review", Medium: "pre_review", High: "block"},
		},
		Rules: config.ModerationRulesConfig{MaxPatternChars: 500, MaxEnabledRegexRules: 200, RequireNonEmptyInEnforce: true},
		Content: config.ModerationContentConfig{
			MomentMaxChars: 800, CommentMaxChars: 2000, GuestbookMaxChars: 2000, ReplyMaxChars: 2000,
			MaxImagesPerContent: 9, MaxLinksPerContent: 10,
		},
		Image: config.ModerationImageConfig{
			MaxUploadBytes: 1_048_576, MaxGIFBytes: 307_200, MaxStoredBytes: 512_000, MaxPixels: 12_000_000,
			ProcessingConcurrency: 2, PreviewMaxEdge: 48, StaticPlaceholderKey: "system/moderation/image-review.jpg",
			GIFPlaceholderKey: "system/moderation/gif-review.jpg", ApprovalRetentionDays: 180, TempRetention: 24 * time.Hour,
			OrphanMinAge: 24 * time.Hour, CleanupInterval: 24 * time.Hour, CleanupBatchSize: 500,
		},
		Review: config.ModerationReviewConfig{
			QueueDefaultPageSize: 20, QueueMaxPageSize: 100, ReasonMaxChars: 1000,
		},
		Governance: config.ModerationGovernanceConfig{
			NewToNormal:              config.ModerationPromotionConfig{MinAgeDays: 7, CleanApprovals: 3},
			NormalToTrusted:          config.ModerationPromotionConfig{MinAgeDays: 30, CleanApprovals: 20},
			RestrictedScoreThreshold: 6, RestrictedDuration: 168 * time.Hour,
			CleanApprovalScoreDecay: 1,
			ViolationWeights:        config.ModerationViolationWeightsConfig{Corrected: 1, Rejected: 3, HighRiskBlocked: 5},
		},
		RateLimit: config.ModerationRateLimitConfig{PublishPerMinute: 10, EditPerMinute: 10, TempUploadPerMinute: 10},
		Control: config.ModerationControlConfig{
			DefaultRegistrationMode: "open", DefaultPublishingMode: "open",
			UserHideBatchSize: 200, UserHideMaxItemsPerRequest: 1000,
		},
		Audit: config.ModerationAuditConfig{
			AttemptRetentionDays: 180, ActionLogRetentionDays: 365, ObsoleteRevisionRetentionDays: 365,
			CleanupInterval: 24 * time.Hour, CleanupBatchSize: 500,
		},
		Migration: config.ModerationMigrationConfig{BatchSize: 200},
		Notices: config.ModerationNoticesConfig{
			Approved: "发布成功。", LowSubmitted: "发布成功，内容会被审核。", ReviewRequired: "内容已提交，等待人工审核。",
			HighRejected: "内容存在较高风险，未能发布，请修改后重试。",
		},
	}
}

func TestModerationReviewConfigRequiresPositiveBounds(t *testing.T) {
	cfg := validModerationConfig()
	cfg.Review.ReasonMaxChars = 0

	err := cfg.Validate("test")

	require.ErrorContains(t, err, "moderation.review.reason_max_chars")
}

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
	writeProjectConfigFixture(t, configDir, `
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
`)

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
	writeProjectConfigFixture(t, configDir, `
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
`)

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
	writeProjectConfigFixture(t, configDir, `
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
`)

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
	writeProjectConfigFixture(t, configDir, "")

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
	writeProjectConfigFixture(t, configDir, `
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
`)

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
	t.Setenv("BLOG_ANALYTICS_GEOIP_V4_PATH", "/app/geoip/ip2region_v4.xdb")
	t.Setenv("BLOG_ANALYTICS_GEOIP_V6_PATH", "/app/geoip/ip2region_v6.xdb")
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
	assert.Equal(t, "/app/geoip/ip2region_v4.xdb", cfg.Analytics.GeoIPV4Path)
	assert.Equal(t, "/app/geoip/ip2region_v6.xdb", cfg.Analytics.GeoIPV6Path)
}

// TestLoad_ReadsImageConfig 验证 CDN 回源图片服务配置能解析到结构体。
func TestLoad_ReadsImageConfig(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(cwd))
	})

	configDir := filepath.Join(t.TempDir(), "config")
	writeProjectConfigFixture(t, configDir, `
image:
  originAuthSecret: "origin-secret"
  responseCacheMaxAge: 3600
  defaultQuality: 80
  maxWidth: 2048
`)

	t.Setenv("APP_ENV", "")
	require.NoError(t, os.Chdir(filepath.Dir(configDir)))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "origin-secret", cfg.Image.OriginAuthSecret)
	assert.Equal(t, 3600, cfg.Image.ResponseCacheMaxAge)
	assert.Equal(t, 80, cfg.Image.DefaultQuality)
	assert.Equal(t, 2048, cfg.Image.MaxWidth)
}

func writeProjectConfigFixture(t *testing.T, configDir, localOverride string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config.yaml"),
		[]byte(projectConfigFile(t, "config.yaml")),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config.local.yaml"),
		[]byte(localOverride),
		0o644,
	))
}
