package database

import (
	"time"

	"github.com/vpt/blog-backend/pkg/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewMySQL 初始化 GORM MySQL 连接并配置连接池
func NewMySQL(cfg *config.DBConfig) (*gorm.DB, error) {
	// 固定 Error 级别：不输出慢查询日志，避免生产环境日志量过大
	gormLogger := logger.Default.LogMode(logger.Error)

	// 打开数据库连接，禁止 GORM AutoMigrate 自动创建外键（外键约束由迁移 SQL 管理）
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger:                                   gormLogger,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}

	// 获取底层 *sql.DB 实例，以便设置连接池参数
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 配置连接池：最大活跃连接数、最大空闲连接数、连接最长复用时间
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MaxLifetimeMinutes) * time.Minute)

	return db, nil
}
