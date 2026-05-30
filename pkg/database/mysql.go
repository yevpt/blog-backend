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

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: gormLogger,
		// 外键约束由迁移 SQL 管理，禁止 GORM AutoMigrate 自动创建
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MaxLifetimeMinutes) * time.Minute)

	return db, nil
}
