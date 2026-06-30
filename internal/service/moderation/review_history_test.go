package moderation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	repositorymock "github.com/vpt/blog-backend/internal/repository/moderation/mock"
	"github.com/vpt/blog-backend/internal/service/moderation"
	"go.uber.org/mock/gomock"
)

func TestReviewHistoryNormalizesPaginationAndMapsReadModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	record := pendingReviewRecord()
	reason := "人工通过"
	repo.EXPECT().LoadReviewHistory(gomock.Any(), uint64(10), 1, 100).Return(moderationrepo.ReviewHistoryPage{
		Total: 1, Page: 1, PageSize: 100,
		Revisions: []moderationrepo.ReviewRecord{record},
		Images: map[uint64][]moderationrepo.RevisionImageRecord{
			record.RevisionID: {{Seq: 0, ObjectKey: "moderation/history/moments/10/a.jpg", MediaType: "image/jpeg"}},
		},
		Events: []moderationrepo.ReviewHistoryEvent{{
			ID: 30, RevisionID: &record.RevisionID, ActorUserID: uint64Ptr(1),
			Action: moderationrepo.EventApprove, Reason: &reason, CreatedAt: serviceNow,
		}},
	}, nil)
	service := newReviewService(repo, &processorStub{})

	page, err := service.History(context.Background(), moderation.ReviewHistoryCommand{
		ItemID: 10, Page: 0, PageSize: 101,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 100, page.PageSize)
	require.Len(t, page.Revisions, 1)
	assert.Equal(t, record.RevisionID, page.Revisions[0].RevisionID)
	require.Len(t, page.Images[record.RevisionID], 1)
	assert.Equal(t, "moderation/history/moments/10/a.jpg", page.Images[record.RevisionID][0].ObjectKey)
	require.Len(t, page.Events, 1)
	assert.Equal(t, moderation.EventApprove, page.Events[0].Action)
}

func uint64Ptr(value uint64) *uint64 { return &value }
