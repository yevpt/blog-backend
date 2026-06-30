package moderationrule_test

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestCreateImportStoresQueuedJob(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `moderation_rule_import`").
		WithArgs("rules.csv", "csv", uint64(1024), "moderation/imports/1.csv", uint64(1),
			"fraud", "review", "medium", int32(100), "queued",
			uint64(0), uint64(0), uint64(0), uint64(0),
			nil, nil, uint64(1),
			sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	imp, err := repo.CreateImport(context.Background(), moderationrule.CreateImportCommand{
		FileName:         "rules.csv",
		Format:           "csv",
		FileSize:         1024,
		ObjectKey:        "moderation/imports/1.csv",
		SourceID:         1,
		DefaultCategory:  "fraud",
		DefaultEffect:    "review",
		DefaultRiskLevel: "medium",
		DefaultPriority:  100,
		OperatorID:       1,
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(1), imp.ID)
	assert.Equal(t, "queued", imp.ValidationStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimNextImportLocksAndUpdatesToValidating(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `moderation_rule_import` WHERE validation_status = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("queued", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "file_name", "format", "file_size", "object_key", "source_id",
			"default_category", "default_effect", "default_risk_level", "default_priority",
			"validation_status", "total_rows", "valid_rows", "duplicate_rows", "error_rows",
			"error_object_key", "ruleset_id", "operator_id", "created_at", "updated_at"}).
			AddRow(5, "rules.csv", "csv", 1024, "key", 1, "fraud", "review", "medium", 100,
				"queued", 0, 0, 0, 0, nil, nil, 1, time.Now(), time.Now()))
	mock.ExpectExec("UPDATE `moderation_rule_import` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	imp, err := repo.ClaimNextImport(context.Background(), time.Now())

	require.NoError(t, err)
	require.NotNil(t, imp)
	assert.Equal(t, uint64(5), imp.ID)
	assert.Equal(t, "validating", imp.ValidationStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimNextImportReturnsNilWhenNoQueued(t *testing.T) {
	observer := &gormErrorObserver{}
	repo, mock := newManagementRepositoryWithLogger(t, observer)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `moderation_rule_import` WHERE validation_status = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("queued", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	imp, err := repo.ClaimNextImport(context.Background(), time.Now())

	require.NoError(t, err)
	assert.Nil(t, imp)
	assert.False(t, observer.Contains(gorm.ErrRecordNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimNextImportReturnsDatabaseError(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `moderation_rule_import` WHERE validation_status = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("queued", 1).
		WillReturnError(errors.New("db down"))
	mock.ExpectRollback()

	imp, err := repo.ClaimNextImport(context.Background(), time.Now())

	assert.Nil(t, imp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResetInterruptedImportsSetsQueued(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `moderation_rule_import` SET").
		WithArgs(sqlmock.AnyArg(), "queued", "validating").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	count, err := repo.ResetInterruptedImports(context.Background(), time.Now())

	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	require.NoError(t, mock.ExpectationsWereMet())
}

type gormErrorObserver struct {
	mu   sync.Mutex
	errs []error
}

func (o *gormErrorObserver) LogMode(gormlogger.LogLevel) gormlogger.Interface { return o }
func (o *gormErrorObserver) Info(context.Context, string, ...any)             {}
func (o *gormErrorObserver) Warn(context.Context, string, ...any)             {}
func (o *gormErrorObserver) Error(context.Context, string, ...any)            {}
func (o *gormErrorObserver) Trace(_ context.Context, _ time.Time, _ func() (string, int64), err error) {
	if err == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.errs = append(o.errs, err)
}

func (o *gormErrorObserver) Contains(target error) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, err := range o.errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
