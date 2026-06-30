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
	mock.ExpectQuery("SELECT .* FROM `moderation_ruleset` WHERE status = .*ORDER BY id DESC LIMIT").
		WithArgs("published", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "index_object_key", "index_format_version", "index_sha256", "index_bytes"}).
			AddRow(1, "published", nil, 1, nil, 0))
	mock.ExpectQuery("SELECT .* FROM moderation_rule AS rule JOIN moderation_ruleset AS activation").
		WithArgs("published", "superseded", uint64(1), uint64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "rule_type", "pattern", "risk_level", "effect", "priority"}).
			AddRow(1, "keyword", "测试风险词", "medium", "review", 1))

	service, err := newModerationService(context.Background(), gdb, config.ModerationConfig{}, zap.NewNop(), nil)

	require.NoError(t, err)
	require.NotNil(t, service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewModerationServiceFailsClosedWhenInitialRuleLoadFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT .* FROM `moderation_ruleset`").WillReturnError(errors.New("database unavailable"))

	service, err := newModerationService(context.Background(), gdb, config.ModerationConfig{}, zap.NewNop(), nil)

	require.Error(t, err)
	require.Nil(t, service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMaybeNewModerationServiceSkipsRuntimeWhenDisabled(t *testing.T) {
	service, err := maybeNewModerationService(context.Background(), nil, config.ModerationConfig{Enabled: false}, zap.NewNop(), nil)

	require.NoError(t, err)
	require.Nil(t, service)
}
