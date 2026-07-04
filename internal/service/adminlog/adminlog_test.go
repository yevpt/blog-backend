package adminlog_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/model"
	repomock "github.com/vpt/blog-backend/internal/repository/adminlog/mock"
	adminlog "github.com/vpt/blog-backend/internal/service/adminlog"
)

func TestService_Record_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repomock.NewMockRepository(ctrl)

	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, entry *model.AdminOperationLog) error {
			assert.Equal(t, uint(1), entry.OperatorID)
			assert.Equal(t, uint(7), entry.TargetUserID)
			assert.Equal(t, string(adminlog.ActionGrantVIP), entry.Action)
			require.NotNil(t, entry.Detail)
			return nil
		},
	)

	svc := adminlog.NewService(repo)
	err := svc.Record(context.Background(), 1, 7, adminlog.ActionGrantVIP, map[string]any{"note": "test"})
	require.NoError(t, err)
}

func TestService_Record_NilDetail(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repomock.NewMockRepository(ctrl)

	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, entry *model.AdminOperationLog) error {
			assert.Nil(t, entry.Detail)
			return nil
		},
	)

	svc := adminlog.NewService(repo)
	err := svc.Record(context.Background(), 1, 7, adminlog.ActionDisableAccount, nil)
	require.NoError(t, err)
}
