package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config 是整个项目的配置结构体，字段与 config.yaml 一一对应
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Log     LogConfig     `mapstructure:"log"`
	JWT     JWTConfig     `mapstructure:"jwt"`
	DB      DBConfig      `mapstructure:"db"`
	Redis   RedisConfig   `mapstructure:"redis"`
	Storage StorageConfig `mapstructure:"storage"`
	Migrate MigrateConfig `mapstructure:"migrate"`
	Email   EmailConfig   `mapstructure:"email"`
}

// MigrateConfig 数据迁移工具专用配置，仅在 config.local.yaml 中设置，不提交到版本库
type MigrateConfig struct {
	SrcDSN string `mapstructure:"src_dsn"` // 源数据库 DSN（只读）
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug / release
}

type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug / info / warn / error
	Format string `mapstructure:"format"` // json / console
}

type JWTConfig struct {
	Secret             string `mapstructure:"secret"`
	ExpireHours        int    `mapstructure:"expire_hours"`
	RefreshExpireHours int    `mapstructure:"refresh_expire_hours"`
}

type DBConfig struct {
	Host                string `mapstructure:"host"`
	Port                int    `mapstructure:"port"`
	Name                string `mapstructure:"name"`
	User                string `mapstructure:"user"`
	Password            string `mapstructure:"password"`
	MaxOpenConns        int    `mapstructure:"max_open_conns"`
	MaxIdleConns        int    `mapstructure:"max_idle_conns"`
	MaxLifetimeMinutes  int    `mapstructure:"max_lifetime_minutes"`
}

// DSN 生成 GORM 连接字符串
func (d *DBConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type StorageConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	Bucket    string `mapstructure:"bucket"`
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
}

type EmailConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	From     string `mapstructure:"from"`
	Password string `mapstructure:"password"`
}

// Load 加载配置，按优先级叠加：config.yaml → config.{env}.yaml → config.local.yaml → 环境变量
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath("../config") // 兼容测试时的路径

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取基础配置失败: %w", err)
	}

	// 加载环境特定配置（dev / prod），如果存在则覆盖基础配置
	env := os.Getenv("APP_ENV")
	if env != "" {
		mergeConfig(v, fmt.Sprintf("config.%s", env))
	}

	// 加载本地覆盖配置（优先级最高，用于本地开发密码等）
	mergeConfig(v, "config.local")

	// 允许通过环境变量覆盖任意配置（前缀 BLOG_，点号用下划线替代）
	// 例如：BLOG_DB_PASSWORD 对应 db.password
	v.SetEnvPrefix("BLOG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置结构体失败: %w", err)
	}

	return &cfg, nil
}

// mergeConfig 尝试合并一个可选的配置文件，文件不存在时静默忽略
func mergeConfig(v *viper.Viper, name string) {
	override := viper.New()
	override.SetConfigName(name)
	override.SetConfigType("yaml")
	override.AddConfigPath("./config")
	override.AddConfigPath("../config")

	if err := override.ReadInConfig(); err != nil {
		return // 文件不存在属于正常情况，不报错
	}

	// 将覆盖配置中的所有键值合并到主配置
	for _, key := range override.AllKeys() {
		v.Set(key, override.Get(key))
	}
}
