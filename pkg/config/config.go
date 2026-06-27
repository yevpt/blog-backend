package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 是整个项目的配置结构体，字段与 config.yaml 一一对应
type Config struct {
	Server ServerConfig `mapstructure:"server"` // HTTP 服务配置
	Log    LogConfig    `mapstructure:"log"`    // 日志输出配置
	JWT    JWTConfig    `mapstructure:"jwt"`    // JWT 签名与过期配置
	DB     DBConfig     `mapstructure:"db"`     // MySQL 数据库配置
	Redis  RedisConfig  `mapstructure:"redis"`  // Redis 连接配置
	Garage GarageConfig `mapstructure:"garage"` // Garage/S3 对象存储配置
	CDN    CDNConfig    `mapstructure:"cdn"`    // CDN 私有读签名配置
	Email  EmailConfig  `mapstructure:"email"`  // 邮件发送配置
	OAuth  OAuthConfig  `mapstructure:"oauth"`  // 第三方 OAuth 登录配置
	// Moderation 是内容审核策略、安全边界和提示配置。
	Moderation ModerationConfig `mapstructure:"moderation"`

	Analytics AnalyticsConfig `mapstructure:"analytics"` // 站点统计采集与聚合配置
}

type ServerConfig struct {
	Port int    `mapstructure:"port"` // HTTP 监听端口
	Mode string `mapstructure:"mode"` // Gin 运行模式：debug / release
}

type LogConfig struct {
	Level  string `mapstructure:"level"`  // 日志级别：debug / info / warn / error
	Format string `mapstructure:"format"` // 日志格式：json / console
}

type JWTConfig struct {
	Secret             string `mapstructure:"secret"`               // JWT 签名密钥
	ExpireHours        int    `mapstructure:"expire_hours"`         // access token 过期小时数
	RefreshExpireHours int    `mapstructure:"refresh_expire_hours"` // refresh token 过期小时数
}

type DBConfig struct {
	Host               string `mapstructure:"host"`                 // MySQL 主机地址
	Port               int    `mapstructure:"port"`                 // MySQL 端口
	Name               string `mapstructure:"name"`                 // 数据库名称
	User               string `mapstructure:"user"`                 // 数据库用户名
	Password           string `mapstructure:"password"`             // 数据库密码
	MaxOpenConns       int    `mapstructure:"max_open_conns"`       // 最大打开连接数
	MaxIdleConns       int    `mapstructure:"max_idle_conns"`       // 最大空闲连接数
	MaxLifetimeMinutes int    `mapstructure:"max_lifetime_minutes"` // 连接最大存活分钟数
}

// DSN 生成 GORM 连接字符串
func (d *DBConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`     // Redis 地址
	Password string `mapstructure:"password"` // Redis 密码
	DB       int    `mapstructure:"db"`       // Redis DB 编号
}

// GarageConfig 是 Garage/S3 兼容对象存储配置。
type GarageConfig struct {
	Endpoint        string `mapstructure:"endpoint"`        // S3 API 地址
	Bucket          string `mapstructure:"bucket"`          // 默认存储桶
	Region          string `mapstructure:"region"`          // S3 签名区域
	AccessKeyID     string `mapstructure:"accessKeyID"`     // 访问密钥 ID
	SecretAccessKey string `mapstructure:"secretAccessKey"` // 访问密钥 Secret
	CDN             bool   `mapstructure:"cdn"`             // 是否优先返回 CDN 签名 URL
}

// CDNConfig 是私有 CDN TypeD 签名配置。
type CDNConfig struct {
	Host               string `mapstructure:"host"`               // CDN 访问域名
	Secret             string `mapstructure:"secret"`             // CDN 签名密钥
	SignQueryName      string `mapstructure:"signQueryName"`      // 签名参数名
	TimestampQueryName string `mapstructure:"timestampQueryName"` // 时间戳参数名
}

type EmailConfig struct {
	Host     string `mapstructure:"host"`      // SMTP 主机地址
	Port     int    `mapstructure:"port"`      // SMTP 端口
	From     string `mapstructure:"from"`      // 发件人邮箱
	Password string `mapstructure:"password"`  // 邮箱授权码或密码
	FromName string `mapstructure:"from_name"` // 发件人昵称，如 YEVPT，为空时仅显示邮箱地址
	SiteURL  string `mapstructure:"site_url"`  // 站点公网访问前缀，用于邮件正文中的跳转链接，如 https://www.example.com

	Provider               string `mapstructure:"provider"`                  // 邮件供应商标识，如 aliyun_enterprise
	ProviderDailyHardLimit int    `mapstructure:"provider_daily_hard_limit"` // 供应商标称每日上限，仅作保护参考
	SiteDailySafeLimit     int    `mapstructure:"site_daily_safe_limit"`     // 站点每日真实安全上限，低于供应商上限
	MaxPerMinute           int    `mapstructure:"max_per_minute"`            // 每分钟发送上限
	MaxPerHour             int    `mapstructure:"max_per_hour"`              // 每小时发送上限
	SendIntervalSeconds    int    `mapstructure:"send_interval_seconds"`     // 单封邮件之间的最小间隔秒数
	WorkerEnabled          bool   `mapstructure:"worker_enabled"`            // 是否启动 dispatcher/sender 等后台 worker
	PlannerEnabled         bool   `mapstructure:"planner_enabled"`           // 是否启动邮件聚合 planner
	WorkerBatchSize        int    `mapstructure:"worker_batch_size"`         // worker 单次领取任务数量
	LeaseSeconds           int    `mapstructure:"lease_seconds"`             // worker 任务租约秒数，超期可被其他实例重新领取
}

// OAuthConfig 是第三方登录总配置，按平台名组织 provider。
type OAuthConfig struct {
	StateTTLMinutes int                            `mapstructure:"state_ttl_minutes"` // state 和 PKCE verifier 在 Redis 中的有效分钟数
	Providers       map[string]OAuthProviderConfig `mapstructure:"providers"`         // 平台配置，key 使用 github/gitee/google 等小写标识
}

// OAuthProviderConfig 是单个第三方平台的 OAuth 配置。
type OAuthProviderConfig struct {
	Enabled      bool     `mapstructure:"enabled"`       // 是否启用该平台
	ClientID     string   `mapstructure:"client_id"`     // OAuth client id，生产环境用环境变量或本地配置覆盖
	ClientSecret string   `mapstructure:"client_secret"` // OAuth client secret，不提交真实值
	RedirectURI  string   `mapstructure:"redirect_uri"`  // 后端 callback 地址，需与平台后台精确一致
	Scopes       []string `mapstructure:"scopes"`        // 授权范围，尽量保持最小权限
	AuthURL      string   `mapstructure:"auth_url"`      // 授权端点
	TokenURL     string   `mapstructure:"token_url"`     // 换取 access token 的端点
	UserURL      string   `mapstructure:"user_url"`      // 获取用户资料的端点
	OpenIDURL    string   `mapstructure:"openid_url"`    // 获取 OpenID 的端点，仅 QQ 等两段式平台使用
	BridgeURL    string   `mapstructure:"bridge_url"`    // 海外 OAuth Bridge 地址，与 bridge_secret 同时配置才生效
	BridgeSecret string   `mapstructure:"bridge_secret"` // Bridge HMAC 共享密钥
	BridgeMode   string   `mapstructure:"bridge_mode"`   // direct | fallback | bridge_only，默认 direct
}

// AnalyticsConfig 是站点统计的采集、实时与聚合配置。
type AnalyticsConfig struct {
	Timezone           string        `mapstructure:"timezone"`             // 切天时区，如 Asia/Shanghai
	RetentionDays      int           `mapstructure:"retention_days"`       // 原始事件保留天数，超期清理
	OnlineWindow       time.Duration `mapstructure:"online_window"`        // 在线判定窗口，如 90s
	SessionTimeout     time.Duration `mapstructure:"session_timeout"`      // 会话超时时长，如 30m
	BounceDuration     time.Duration `mapstructure:"bounce_duration"`      // 跳出判定停留阈值，如 10s
	ChannelBuffer      int           `mapstructure:"channel_buffer"`       // 异步落库 channel 缓冲大小
	PublicCacheTTL     time.Duration `mapstructure:"public_cache_ttl"`     // 公开统计接口缓存 TTL，如 60s
	GeoIPV4Path        string        `mapstructure:"geoip_v4_path"`        // ip2region IPv4 xdb 路径，空则关闭 IPv4 地理解析
	GeoIPV6Path        string        `mapstructure:"geoip_v6_path"`        // ip2region IPv6 xdb 路径，空则关闭 IPv6 地理解析
	SiteHost           string        `mapstructure:"site_host"`            // 站点主域名，用于来源/外链判定
	IPSalt             string        `mapstructure:"ip_salt"`              // IP 哈希盐，生产经 env 覆盖为随机串
	CollectTokenSecret string        `mapstructure:"collect_token_secret"` // /collect HMAC token secret，空则开发放行
	CollectTokenTTL    time.Duration `mapstructure:"collect_token_ttl"`    // collect token 有效期，如 5m
}

// Load 按优先级叠加加载配置：config.yaml → config.{APP_ENV}.yaml → config.local.yaml → 环境变量（BLOG_ 前缀）
func Load() (*Config, error) {
	v := viper.New()
	// 设置基础配置文件名和搜索路径，同时支持项目根目录和上级目录（兼容测试工作目录）
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath("../config")
	v.AddConfigPath("../../config")    // 支持从 pkg/xxx/ 目录运行测试
	v.AddConfigPath("../../../config") // 支持从 internal/xxx/yyy/ 目录运行测试

	// 读取基础配置，失败时阻断启动（必要文件缺失无法继续运行）
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取基础配置失败: %w", err)
	}

	// 按运行环境叠加对应的环境配置（如 APP_ENV=prod 则叠加 config.prod.yaml）
	env := os.Getenv("APP_ENV")
	if env != "" {
		mergeConfig(v, fmt.Sprintf("config.%s", env))
	}

	// 叠加本地开发配置（敏感凭证不提交版本库，通过 config.local.yaml 覆盖）
	mergeConfig(v, "config.local")

	// 环境变量优先级最高，点号层级用下划线替代，例如 BLOG_DB_PASSWORD → db.password
	v.SetEnvPrefix("BLOG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	bindRuntimeEnv(v)
	v.AutomaticEnv()

	// 将最终合并后的配置反序列化到结构体
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置结构体失败: %w", err)
	}

	// 使用实际 APP_ENV 校验最终叠加结果，避免覆盖配置绕过生产审核约束。
	if err := cfg.Moderation.Validate(env); err != nil {
		return nil, fmt.Errorf("校验审核配置失败: %w", err)
	}

	return &cfg, nil
}

func bindRuntimeEnv(v *viper.Viper) {
	// 只存在于生产环境变量中的 key 需要显式绑定，否则 Unmarshal 无法发现这些字段。
	keys := []string{
		"server.port",
		"server.mode",
		"log.level",
		"log.format",
		"jwt.secret",
		"jwt.expire_hours",
		"jwt.refresh_expire_hours",
		"db.host",
		"db.port",
		"db.name",
		"db.user",
		"db.password",
		"db.max_open_conns",
		"db.max_idle_conns",
		"db.max_lifetime_minutes",
		"redis.addr",
		"redis.password",
		"redis.db",
		"garage.endpoint",
		"garage.bucket",
		"garage.region",
		"garage.accessKeyID",
		"garage.secretAccessKey",
		"garage.cdn",
		"cdn.host",
		"cdn.secret",
		"cdn.signQueryName",
		"cdn.timestampQueryName",
		"email.host",
		"email.port",
		"email.from",
		"email.password",
		"email.provider",
		"email.provider_daily_hard_limit",
		"email.site_daily_safe_limit",
		"email.max_per_minute",
		"email.max_per_hour",
		"email.send_interval_seconds",
		"email.worker_enabled",
		"email.planner_enabled",
		"email.worker_batch_size",
		"email.lease_seconds",
		"oauth.state_ttl_minutes",
		"analytics.ip_salt",
		"analytics.geoip_v4_path",
		"analytics.geoip_v6_path",
		"analytics.site_host",
		"analytics.collect_token_secret",
	}

	for _, key := range keys {
		_ = v.BindEnv(key)
	}
}

// mergeConfig 将可选配置文件的所有键值叠加到主配置，文件不存在时静默忽略
func mergeConfig(v *viper.Viper, name string) {
	// 创建独立 viper 实例加载覆盖配置，避免与主配置实例互相干扰
	override := viper.New()
	override.SetConfigName(name)
	override.SetConfigType("yaml")
	override.AddConfigPath("./config")
	override.AddConfigPath("../config")
	override.AddConfigPath("../../config")
	override.AddConfigPath("../../../config")

	// 文件不存在时静默忽略，所有覆盖配置文件均为可选
	if err := override.ReadInConfig(); err != nil {
		return
	}

	// 将覆盖配置的所有键逐一写入主配置，实现增量叠加（后者覆盖前者）
	for _, key := range override.AllKeys() {
		v.Set(key, override.Get(key))
	}
}
