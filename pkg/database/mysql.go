package database

import (
	"time"

	"github.com/vpt/blog-backend/pkg/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewMySQL 根据配置初始化 GORM MySQL 连接
func NewMySQL(cfg *config.DBConfig) (*gorm.DB, error) {
	// gorm 日志级别：生产环境只记录 Error，开发环境记录所有 SQL
	gormLogger := logger.Default.LogMode(logger.Error)

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: gormLogger,
		// 禁用自动创建外键约束，表结构由迁移 SQL 管理
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}

	// 获取底层 sql.DB 设置连接池参数
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MaxLifetimeMinutes) * time.Minute)

	return db, nil
}
