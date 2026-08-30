package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	userrepo "github.com/vpt/blog-backend/internal/repository/user"
)

func TestAuthenticationRepositoryFindByEmailUsesContext(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := userrepo.NewAuthenticationRepository(db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := repo.FindByEmail(ctx, "alice@example.com")

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Zero(t, result.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthenticationRepositoryExistsByEmail(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := userrepo.NewAuthenticationRepository(db)

	mock.ExpectQuery(`SELECT count\(\*\) FROM \x60user\x60`).
		WithArgs("alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	exists, err := repo.ExistsByEmail(context.Background(), "alice@example.com")

	require.NoError(t, err)
	assert.True(t, exists)
	assert.NoError(t, mock.ExpectationsWereMet())
}
