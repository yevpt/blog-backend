package router

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestNewModerationServiceLoadsRulesBeforeReturningRuntime(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT .* FROM `moderation_rule` WHERE enabled = .*ORDER BY priority ASC,id ASC").
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rule_type", "pattern", "risk_level", "priority", "ruleset_version", "enabled"}).
			AddRow(1, "基础规则", "keyword", "测试风险词", "medium", 1, 1, true))

	service, err := newModerationService(context.Background(), gdb, config.ModerationConfig{}, zap.NewNop())

	require.NoError(t, err)
	require.NotNil(t, service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewModerationServiceFailsClosedWhenInitialRuleLoadFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT .* FROM `moderation_rule`").WillReturnError(errors.New("database unavailable"))

	service, err := newModerationService(context.Background(), gdb, config.ModerationConfig{}, zap.NewNop())

	require.Error(t, err)
	require.Nil(t, service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMaybeNewModerationServiceSkipsRuntimeWhenDisabled(t *testing.T) {
	service, err := maybeNewModerationService(context.Background(), nil, config.ModerationConfig{Enabled: false}, zap.NewNop())

	require.NoError(t, err)
	require.Nil(t, service)
}
