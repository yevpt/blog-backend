package config

import (
	"fmt"
	"strings"
	"time"
)

const minimumProductionSecretLength = 32

// Validate 校验通用运行边界，并在生产环境执行依赖与密钥的严格检查。
func (c Config) Validate(environment string) error {
	if err := validateRuntimeBounds(c); err != nil {
		return err
	}
	if err := c.Moderation.Validate(environment); err != nil {
		return err
	}
	if !isProductionEnvironment(environment) {
		return nil
	}
	return validateProductionConfig(c)
}

func validateRuntimeBounds(c Config) error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port: 必须在 1-65535 之间")
	}
	if c.Server.Mode != "debug" && c.Server.Mode != "release" && c.Server.Mode != "test" {
		return fmt.Errorf("server.mode: 不支持 %q", c.Server.Mode)
	}
	if c.JWT.ExpireHours <= 0 || c.JWT.RefreshExpireHours <= c.JWT.ExpireHours {
		return fmt.Errorf("jwt: refresh_expire_hours 必须大于正数 expire_hours")
	}
	if c.DB.MaxOpenConns <= 0 || c.DB.MaxIdleConns < 0 || c.DB.MaxIdleConns > c.DB.MaxOpenConns {
		return fmt.Errorf("db: 连接池参数无效")
	}
	if c.DB.MaxLifetimeMinutes <= 0 {
		return fmt.Errorf("db.max_lifetime_minutes: 必须为正数")
	}
	if c.Redis.DB < 0 {
		return fmt.Errorf("redis.db: 不能为负数")
	}
	if c.Image.DefaultQuality < 1 || c.Image.DefaultQuality > 100 {
		return fmt.Errorf("image.defaultQuality: 必须在 1-100 之间")
	}
	if c.Image.MaxWidth <= 0 || c.Image.ResponseCacheMaxAge < 0 {
		return fmt.Errorf("image: maxWidth 必须为正数且 responseCacheMaxAge 不能为负数")
	}
	if err := validateEmailBounds(c.Email); err != nil {
		return err
	}
	if err := validateAnalyticsBounds(c.Analytics); err != nil {
		return err
	}
	return validateOAuthConfig(c.OAuth)
}

func validateEmailBounds(c EmailConfig) error {
	if c.ProviderDailyHardLimit <= 0 || c.SiteDailySafeLimit <= 0 || c.SiteDailySafeLimit > c.ProviderDailyHardLimit {
		return fmt.Errorf("email: 每日限额参数无效")
	}
	if c.MaxPerMinute <= 0 || c.MaxPerHour < c.MaxPerMinute {
		return fmt.Errorf("email: 分钟或小时限额参数无效")
	}
	if c.WorkerBatchSize <= 0 || c.LeaseSeconds <= 0 || c.SendIntervalSeconds <= 0 {
		return fmt.Errorf("email: worker 参数必须为正数")
	}
	if c.PlannerEnabled && !c.WorkerEnabled {
		return fmt.Errorf("email.planner_enabled: 依赖 worker_enabled")
	}
	return nil
}

func validateAnalyticsBounds(c AnalyticsConfig) error {
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("analytics.timezone: %w", err)
	}
	if c.RetentionDays <= 0 || c.ChannelBuffer <= 0 {
		return fmt.Errorf("analytics: retention_days 和 channel_buffer 必须为正数")
	}
	if c.OnlineWindow <= 0 || c.SessionTimeout <= 0 || c.BounceDuration <= 0 || c.CollectTokenTTL <= 0 {
		return fmt.Errorf("analytics: 时间参数必须为正数")
	}
	if c.PublicCacheTTL < 0 {
		return fmt.Errorf("analytics.public_cache_ttl: 不能为负数")
	}
	return nil
}

func validateOAuthConfig(c OAuthConfig) error {
	if c.StateTTLMinutes <= 0 {
		return fmt.Errorf("oauth.state_ttl_minutes: 必须为正数")
	}
	for name, provider := range c.Providers {
		if !provider.Enabled {
			continue
		}
		if blank(provider.ClientID, provider.ClientSecret, provider.RedirectURI, provider.AuthURL, provider.TokenURL, provider.UserURL) {
			return fmt.Errorf("oauth.providers.%s: 已启用平台缺少必要配置", name)
		}
		if provider.BridgeMode != "" && provider.BridgeMode != "direct" && provider.BridgeMode != "fallback" && provider.BridgeMode != "bridge_only" {
			return fmt.Errorf("oauth.providers.%s.bridge_mode: 不支持 %q", name, provider.BridgeMode)
		}
	}
	return nil
}

func validateProductionConfig(c Config) error {
	if c.Server.Mode != "release" {
		return fmt.Errorf("server.mode: 生产环境必须为 release")
	}
	if err := requireProductionSecret("jwt.secret", c.JWT.Secret); err != nil {
		return err
	}
	if c.DB.Port < 1 || c.DB.Port > 65535 || blank(c.DB.Host, c.DB.Name, c.DB.User, c.DB.Password) {
		return fmt.Errorf("db: 生产环境必须提供完整连接配置")
	}
	if isPlaceholderSecret(c.DB.Password) {
		return fmt.Errorf("db.password: 生产环境不能使用占位值")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		return fmt.Errorf("redis.addr: 生产环境不能为空")
	}
	if blank(c.Garage.Endpoint, c.Garage.Bucket, c.Garage.Region, c.Garage.AccessKeyID, c.Garage.SecretAccessKey) {
		return fmt.Errorf("garage: 生产环境必须提供完整对象存储配置")
	}
	if isPlaceholderSecret(c.Garage.SecretAccessKey) {
		return fmt.Errorf("garage.secretAccessKey: 生产环境不能使用占位值")
	}
	if err := validateProductionEmail(c.Email); err != nil {
		return err
	}
	if err := requireProductionSecret("analytics.ip_salt", c.Analytics.IPSalt); err != nil {
		return err
	}
	if strings.TrimSpace(c.Analytics.SiteHost) == "" {
		return fmt.Errorf("analytics.site_host: 生产环境不能为空")
	}
	if tokenSecret := strings.TrimSpace(c.Analytics.CollectTokenSecret); tokenSecret != "" && len(tokenSecret) < minimumProductionSecretLength {
		return fmt.Errorf("analytics.collect_token_secret: 设置后长度不能少于 %d 个字符", minimumProductionSecretLength)
	}
	if c.Garage.CDN {
		return validateProductionCDN(c)
	}
	return nil
}

func validateProductionEmail(c EmailConfig) error {
	if c.Port < 1 || c.Port > 65535 || blank(c.Host, c.From, c.Password) {
		return fmt.Errorf("email: 生产环境必须提供完整 SMTP 配置")
	}
	if isPlaceholderSecret(c.Password) {
		return fmt.Errorf("email.password: 生产环境不能使用占位值")
	}
	return nil
}

func validateProductionCDN(c Config) error {
	if blank(c.CDN.Host, c.CDN.Secret, c.CDN.SignQueryName, c.CDN.TimestampQueryName) {
		return fmt.Errorf("cdn: 启用 Garage CDN 时必须提供完整签名配置")
	}
	if c.CDN.SignQueryName == c.CDN.TimestampQueryName {
		return fmt.Errorf("cdn: 签名参数名与时间戳参数名不能相同")
	}
	return requireProductionSecret("image.originAuthSecret", c.Image.OriginAuthSecret)
}

func requireProductionSecret(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < minimumProductionSecretLength || isPlaceholderSecret(trimmed) {
		return fmt.Errorf("%s: 生产密钥必须至少 %d 个字符且不能使用占位值", field, minimumProductionSecretLength)
	}
	return nil
}

func isPlaceholderSecret(value string) bool {
	normalized := strings.ToLower(value)
	return strings.Contains(normalized, "change_me") || strings.Contains(normalized, "change-me") || strings.HasPrefix(normalized, "your_")
}

func isProductionEnvironment(environment string) bool {
	return environment == "prod" || environment == "production"
}

func blank(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}
