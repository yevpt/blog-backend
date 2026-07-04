package user_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/model"
	logmock "github.com/vpt/blog-backend/internal/repository/adminlog/mock"
	"github.com/vpt/blog-backend/internal/repository/user/mock"
	userservice "github.com/vpt/blog-backend/internal/service/user"
)

func TestAdminService_GetOperationLogs(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	logs := logmock.NewMockRepository(ctrl)

	logs.EXPECT().ListByTargetUser(gomock.Any(), uint(7), 0, 10).
		Return([]model.AdminOperationLog{{ID: 1, OperatorID: 1, TargetUserID: 7, Action: "grant_vip"}}, int64(1), nil)

	svc := userservice.NewAdminService(repo, &stubUserCacheService{}, userservice.AdminDeps{Logs: logs})
	resp, err := svc.GetOperationLogs(7, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, "grant_vip", resp.List[0].Action)
}
