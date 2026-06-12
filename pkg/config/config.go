package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config 是整个项目的配置结构体，字段与 config.yaml 一一对应
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`  // HTTP 服务配置
	Log     LogConfig     `mapstructure:"log"`     // 日志输出配置
	JWT     JWTConfig     `mapstructure:"jwt"`     // JWT 签名与过期配置
	DB      DBConfig      `mapstructure:"db"`      // MySQL 数据库配置
	Redis   RedisConfig   `mapstructure:"redis"`   // Redis 连接配置
	Garage  GarageConfig  `mapstructure:"garage"`  // Garage/S3 对象存储配置
	CDN     CDNConfig     `mapstructure:"cdn"`     // CDN 私有读签名配置
	Migrate MigrateConfig `mapstructure:"migrate"` // 数据迁移工具配置
	Email   EmailConfig   `mapstructure:"email"`   // 邮件发送配置
	OAuth   OAuthConfig   `mapstructure:"oauth"`   // 第三方 OAuth 登录配置
}

// MigrateConfig 数据迁移工具专用配置，仅在 config.local.yaml 中设置，不提交到版本库
type MigrateConfig struct {
	SrcDSN string `mapstructure:"src_dsn"` // 源数据库 DSN（只读）
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
	Host     string `mapstructure:"host"`     // SMTP 主机地址
	Port     int    `mapstructure:"port"`     // SMTP 端口
	From     string `mapstructure:"from"`     // 发件人邮箱
	Password string `mapstructure:"password"` // 邮箱授权码或密码
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
		"oauth.state_ttl_minutes",
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
