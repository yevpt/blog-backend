package moderation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	repositorymock "github.com/vpt/blog-backend/internal/repository/moderation/mock"
	"github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

type publisherStub struct {
	called  bool
	command moderationmedia.PublishCommand
	result  moderationmedia.PublishResult
	err     error
}

func (p *publisherStub) Publish(_ context.Context, cmd moderationmedia.PublishCommand) (moderationmedia.PublishResult, error) {
	p.called = true
	p.command = cmd
	return p.result, p.err
}

func serviceCfg() config.ModerationConfig {
	return config.ModerationConfig{
		Enabled: true,
		Content: config.ModerationContentConfig{
			MomentMaxChars: 800, CommentMaxChars: 2000,
			GuestbookMaxChars: 2000, ReplyMaxChars: 2000,
			MaxImagesPerContent: 9, MaxLinksPerContent: 10,
		},
		Notices: config.ModerationNoticesConfig{
			Approved:       "发布成功。",
			LowSubmitted:   "发布成功，内容会被审核。",
			ReviewRequired: "内容已提交，等待人工审核。",
			HighRejected:   "内容存在较高风险，未能发布，请修改后重试。",
		},
		Governance: config.ModerationGovernanceConfig{
			ViolationWeights: config.ModerationViolationWeightsConfig{Corrected: 1, Rejected: 3, HighRiskBlocked: 5},
		},
	}
}

func newServiceWithPublisher(
	repo moderationrepo.Repository,
	media moderation.MediaService,
	publisher moderation.ApprovedImagePublisher,
) moderation.Service {
	return moderation.NewService(repo, &processorStub{}, &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}},
		&deciderStub{action: moderation.ActionAutoApprove}, media, publisher, serviceCfg(), zap.NewNop(), func() time.Time { return serviceNow })
}

func TestAutoApproveMomentWithImagesCallsPublisher(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	media := &mediaServiceStub{set: moderationmedia.PreparedSet{Images: []moderationmedia.PreparedImage{{
		Fingerprint: moderationmedia.Fingerprint{SHA256: "sha", MD5: "md5", Size: 10},
		ObjectKey:   "moderation/staging/moments/7/u1/sha.jpg", MediaType: "image/jpeg",
	}}}}
	publisher := &publisherStub{}
	cmd := moderation.SubmitCommand{
		ActorID: 7, Subject: moderation.SubjectRef{Type: moderation.SubjectMoment},
		Content: "碎语带图", IdempotencyKey: "moment-auto-1",
		ImageKeys: []string{"moderation/staging/moments/7/u1/sha.jpg"},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).Return(moderationrepo.AppliedTransition{
			Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectMoment, ID: 88},
			ItemID:  10, RevisionID: 20, RevisionVersion: 1, LockVersion: 2,
		}, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), uint64(20)).Return([]moderationrepo.RevisionImageRecord{{
			ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: "sha", MD5: "md5", Size: 10},
			Seq:              1, ObjectKey: "moderation/staging/moments/7/u1/sha.jpg", MediaType: "image/jpeg",
		}}, nil),
	)

	_, err := newServiceWithPublisher(repo, media, publisher).Submit(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, publisher.called)
	assert.Equal(t, uint64(10), publisher.command.ItemID)
	assert.Equal(t, uint64(20), publisher.command.RevisionID)
	assert.Equal(t, uint64(7), publisher.command.UserID)
	assert.Equal(t, uint64(88), publisher.command.MomentID)
	require.Len(t, publisher.command.Current, 1)
	assert.Equal(t, "moderation/staging/moments/7/u1/sha.jpg", publisher.command.Current[0].ObjectKey)
}

func TestAutoApprovePublisherFailureRevertsProjectionAndReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	media := &mediaServiceStub{set: moderationmedia.PreparedSet{Images: []moderationmedia.PreparedImage{{
		Fingerprint: moderationmedia.Fingerprint{SHA256: "sha", MD5: "md5", Size: 10},
		ObjectKey:   "moderation/staging/moments/7/u1/sha.jpg", MediaType: "image/jpeg",
	}}}}
	publisher := &publisherStub{err: errors.New("garage down")}
	cmd := moderation.SubmitCommand{
		ActorID: 7, Subject: moderation.SubjectRef{Type: moderation.SubjectMoment},
		Content: "碎语带图", IdempotencyKey: "moment-auto-fail",
		ImageKeys: []string{"moderation/staging/moments/7/u1/sha.jpg"},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).Return(moderationrepo.AppliedTransition{
			Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectMoment, ID: 88},
			ItemID:  10, RevisionID: 20, RevisionVersion: 1, LockVersion: 2,
		}, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), uint64(20)).Return([]moderationrepo.RevisionImageRecord{{
			ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: "sha", MD5: "md5", Size: 10},
			Seq:              1, ObjectKey: "moderation/staging/moments/7/u1/sha.jpg", MediaType: "image/jpeg",
		}}, nil),
		repo.EXPECT().RevertPublicProjection(gomock.Any(), uint64(10), uint64(88)).Return(nil),
	)

	_, err := newServiceWithPublisher(repo, media, publisher).Submit(context.Background(), cmd)

	require.Error(t, err)
	require.True(t, publisher.called)
}

func TestReviewApproveMomentWithImagesCallsPublisher(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	publisher := &publisherStub{}
	record := pendingMomentReviewRecord()
	record.ReviewStatus = moderationrepo.ReviewPending
	gomock.InOrder(
		repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), record.RevisionID).Return([]moderationrepo.RevisionImageRecord{{
			ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: "sha", MD5: "md5", Size: 10},
			Seq:              1, ObjectKey: "moderation/staging/moments/42/u1/sha.jpg", MediaType: "image/jpeg",
		}}, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), record.State.Materialized.ID).Return([]moderationrepo.RevisionImageRecord{{
			ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: "oldsha", MD5: "oldmd5", Size: 10},
			Seq:              1, ObjectKey: "moments/42/8/oldsha.jpg", MediaType: "image/jpeg",
		}}, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), record.Subject).Return(
			moderationrepo.SubjectSnapshot{Ref: record.Subject, AuthorID: record.AuthorID, Content: record.PublishedContent}, nil,
		),
		repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), gomock.Any()).Return(
			moderationrepo.ReviewNotificationContext{ContentType: moderationrepo.SubjectMoment}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).Return(moderationrepo.AppliedTransition{
			Subject: record.Subject, ItemID: record.ItemID, LockVersion: 4,
		}, nil),
	)
	service := newReviewServiceWithPublisher(repo, publisher)

	_, err := service.Approve(context.Background(), moderation.ReviewCommand{
		ItemID: record.ItemID, RevisionID: record.RevisionID,
		ExpectedLockVersion: record.LockVersion, ReviewerID: 1,
	})

	require.NoError(t, err)
	require.True(t, publisher.called)
	assert.Equal(t, record.ItemID, publisher.command.ItemID)
	assert.Equal(t, record.RevisionID, publisher.command.RevisionID)
	assert.Equal(t, record.Subject.ID, publisher.command.MomentID)
	require.Len(t, publisher.command.Current, 1)
	require.Len(t, publisher.command.Previous, 1)
	assert.Equal(t, "moments/42/8/oldsha.jpg", publisher.command.Previous[0].ObjectKey)
}

func TestReviewCorrectMomentWithImagesCallsPublisher(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	publisher := &publisherStub{}
	record := pendingMomentReviewRecord()
	record.ReviewStatus = moderationrepo.ReviewPending
	gomock.InOrder(
		repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), record.RevisionID).Return([]moderationrepo.RevisionImageRecord{{
			ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: "sha", MD5: "md5", Size: 10},
			Seq:              1, ObjectKey: "moderation/staging/moments/42/u1/sha.jpg", MediaType: "image/jpeg",
		}}, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), record.State.Materialized.ID).Return(nil, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), record.Subject).Return(
			moderationrepo.SubjectSnapshot{Ref: record.Subject, AuthorID: record.AuthorID, Content: record.PublishedContent}, nil,
		),
		repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), gomock.Any()).Return(
			moderationrepo.ReviewNotificationContext{ContentType: moderationrepo.SubjectMoment}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).Return(moderationrepo.AppliedTransition{
			Subject: record.Subject, ItemID: record.ItemID, LockVersion: 4,
		}, nil),
	)
	processor := &processorStub{useOut: true, out: moderation.ProcessedContent{
		Published: "<p>修正碎语</p>", PlainText: "修正碎语",
	}}
	service := newReviewServiceWithPublisherAndProcessor(repo, publisher, processor)

	_, err := service.Correct(context.Background(), moderation.CorrectCommand{
		ReviewCommand: moderation.ReviewCommand{
			ItemID: record.ItemID, RevisionID: record.RevisionID,
			ExpectedLockVersion: record.LockVersion, ReviewerID: 1, Reason: "修正图片",
		},
		Content: "<p>修正碎语</p>",
	})

	require.NoError(t, err)
	require.True(t, publisher.called)
	assert.Equal(t, record.Subject.ID, publisher.command.MomentID)
}

func TestReviewApprovePublisherFailureRevertsProjection(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	publisher := &publisherStub{err: errors.New("garage down")}
	record := pendingMomentReviewRecord()
	record.ReviewStatus = moderationrepo.ReviewPending
	gomock.InOrder(
		repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), record.RevisionID).Return([]moderationrepo.RevisionImageRecord{{
			ImageFingerprint: moderationrepo.ImageFingerprint{SHA256: "sha", MD5: "md5", Size: 10},
			Seq:              1, ObjectKey: "moderation/staging/moments/42/u1/sha.jpg", MediaType: "image/jpeg",
		}}, nil),
		repo.EXPECT().LoadRevisionImages(gomock.Any(), record.State.Materialized.ID).Return(nil, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), record.Subject).Return(
			moderationrepo.SubjectSnapshot{Ref: record.Subject, AuthorID: record.AuthorID, Content: record.PublishedContent}, nil,
		),
		repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), gomock.Any()).Return(
			moderationrepo.ReviewNotificationContext{ContentType: moderationrepo.SubjectMoment}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).Return(moderationrepo.AppliedTransition{
			Subject: record.Subject, ItemID: record.ItemID, LockVersion: 4,
		}, nil),
		repo.EXPECT().RevertPublicProjection(gomock.Any(), record.ItemID, record.Subject.ID).Return(nil),
	)
	service := newReviewServiceWithPublisher(repo, publisher)

	_, err := service.Approve(context.Background(), moderation.ReviewCommand{
		ItemID: record.ItemID, RevisionID: record.RevisionID,
		ExpectedLockVersion: record.LockVersion, ReviewerID: 1,
	})

	require.Error(t, err)
	require.True(t, publisher.called)
}

func pendingMomentReviewRecord() moderationrepo.ReviewRecord {
	return moderationrepo.ReviewRecord{
		ItemID: 10, Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectMoment, ID: 8},
		AuthorID: 42, LockVersion: 3,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
			Materialized: moderationrepo.ExistingRevision(15), Approved: moderationrepo.ExistingRevision(15),
			Pending: moderationrepo.ExistingRevision(20),
		},
		RevisionID: 20, RevisionVersion: 2, SubmittedContent: "碎语原文", PublishedContent: "碎语正文",
		RiskLevel: moderationrepo.RiskLow, PolicyAction: moderationrepo.ActionPreReview,
		ReviewStatus: moderationrepo.ReviewPending, CreatedAt: serviceNow,
	}
}

func newReviewServiceWithPublisher(repo moderationrepo.Repository, publisher moderation.ApprovedImagePublisher) moderation.ReviewService {
	cfg := config.ModerationConfig{
		Content: config.ModerationContentConfig{
			MomentMaxChars: 800, CommentMaxChars: 2000, GuestbookMaxChars: 2000, ReplyMaxChars: 2000,
		},
		Review: config.ModerationReviewConfig{QueueDefaultPageSize: 20, QueueMaxPageSize: 100, ReasonMaxChars: 1000},
		Governance: config.ModerationGovernanceConfig{
			CleanApprovalScoreDecay: 1,
			ViolationWeights:        config.ModerationViolationWeightsConfig{Corrected: 1, Rejected: 3},
		},
	}
	return moderation.NewReviewService(repo, &processorStub{}, nil, publisher, cfg, zap.NewNop(), func() time.Time { return serviceNow })
}

func newReviewServiceWithPublisherAndProcessor(repo moderationrepo.Repository, publisher moderation.ApprovedImagePublisher, processor moderation.ContentProcessor) moderation.ReviewService {
	cfg := config.ModerationConfig{
		Content: config.ModerationContentConfig{
			MomentMaxChars: 800, CommentMaxChars: 2000, GuestbookMaxChars: 2000, ReplyMaxChars: 2000,
		},
		Review: config.ModerationReviewConfig{QueueDefaultPageSize: 20, QueueMaxPageSize: 100, ReasonMaxChars: 1000},
		Governance: config.ModerationGovernanceConfig{
			CleanApprovalScoreDecay: 1,
			ViolationWeights:        config.ModerationViolationWeightsConfig{Corrected: 1, Rejected: 3},
		},
	}
	return moderation.NewReviewService(repo, processor, nil, publisher, cfg, zap.NewNop(), func() time.Time { return serviceNow })
}
