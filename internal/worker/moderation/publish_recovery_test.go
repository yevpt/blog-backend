package moderation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	moderationworker "github.com/vpt/blog-backend/internal/worker/moderation"
	"go.uber.org/zap"
)

type recoveryRepositoryStub struct {
	rows   []moderationrepo.PublishRecoveryCandidate
	images map[uint64][]moderationrepo.RevisionImageRecord
}

func (s *recoveryRepositoryStub) ListPublishRecoveryCandidates(context.Context, int) ([]moderationrepo.PublishRecoveryCandidate, error) {
	return s.rows, nil
}

func (s *recoveryRepositoryStub) LoadRevisionImages(_ context.Context, revisionID uint64) ([]moderationrepo.RevisionImageRecord, error) {
	return s.images[revisionID], nil
}

type recoveryPublisherStub struct {
	commands []moderationmedia.PublishCommand
}

func (s *recoveryPublisherStub) Publish(_ context.Context, cmd moderationmedia.PublishCommand) (moderationmedia.PublishResult, error) {
	s.commands = append(s.commands, cmd)
	return moderationmedia.PublishResult{}, nil
}

func TestPublishRecoveryWorkerPublishesCurrentAndPreviousRevision(t *testing.T) {
	previousID := uint64(19)
	repo := &recoveryRepositoryStub{
		rows: []moderationrepo.PublishRecoveryCandidate{{
			ItemID: 10, RevisionID: 20, AuthorID: 7, MomentID: 88, PreviousRevisionID: &previousID,
		}},
		images: map[uint64][]moderationrepo.RevisionImageRecord{
			20: {{Seq: 1, ObjectKey: "moderation/staging/moments/7/batch/new.jpg"}},
			19: {{Seq: 1, ObjectKey: "moments/7/88/old.jpg"}},
		},
	}
	publisher := &recoveryPublisherStub{}
	worker := moderationworker.NewPublishRecoveryWorker(repo, publisher, zap.NewNop())

	assert.Equal(t, 1, worker.RecoverOnce(context.Background()))
	require.Len(t, publisher.commands, 1)
	assert.Equal(t, uint64(20), publisher.commands[0].RevisionID)
	require.Len(t, publisher.commands[0].Current, 1)
	require.Len(t, publisher.commands[0].Previous, 1)
}
