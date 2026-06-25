// 数据库初始化工具：按当前代码模型创建表结构，并写入系统默认数据。
//
// 用法：
//
//	go run ./cmd/dbsetup
package main

import (
	"flag"
	"io"
	"log"
	"os"

	"github.com/vpt/blog-backend/internal/dbschema"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/database"
)

type setupOptions struct {
	Seed dbschema.SeedOptions
}

func main() {
	opts, err := parseSetupOptions(os.Args[1:])
	if err != nil {
		log.Fatalf("参数错误: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	db, err := database.NewMySQL(&cfg.DB)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	log.Printf("✓ 数据库连接成功（%s/%s）", cfg.DB.Host, cfg.DB.Name)

	if err := dbschema.AutoMigrate(db); err != nil {
		log.Fatalf("建表失败: %v", err)
	}
	log.Println("✓ 当前表结构已初始化")

	if err := dbschema.SeedDefaults(db, opts.Seed); err != nil {
		log.Fatalf("默认数据初始化失败: %v", err)
	}
	log.Println("✓ 默认数据已初始化：admin 用户固定为 id=1")
}

func parseSetupOptions(args []string) (setupOptions, error) {
	opts := setupOptions{
		Seed: dbschema.SeedOptions{
			AdminUsername: envDefault("BLOG_DBSETUP_ADMIN_USERNAME", "admin"),
			AdminPassword: envDefault("BLOG_DBSETUP_ADMIN_PASSWORD", "admin"),
			AdminEmail:    os.Getenv("BLOG_DBSETUP_ADMIN_EMAIL"),
		},
	}

	fs := flag.NewFlagSet("dbsetup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Seed.AdminUsername, "admin-username", opts.Seed.AdminUsername, "初始管理员用户名")
	fs.StringVar(&opts.Seed.AdminPassword, "admin-password", opts.Seed.AdminPassword, "初始管理员密码")
	fs.StringVar(&opts.Seed.AdminEmail, "admin-email", opts.Seed.AdminEmail, "初始管理员邮箱")
	if err := fs.Parse(args); err != nil {
		return setupOptions{}, err
	}

	return opts, nil
}

func envDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
