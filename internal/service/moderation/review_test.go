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
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

type previewCleanerStub struct {
	keys []string
}

func (s *previewCleanerStub) DeletePreviewObjects(_ context.Context, keys []string) error {
	s.keys = append([]string(nil), keys...)
	return nil
}

func TestReviewServiceApproveBuildsApprovedTransition(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	record := pendingReviewRecord()
	parentID := uint64(16)
	record.Subject = moderationrepo.SubjectRef{
		Type: moderationrepo.SubjectArticleCommentReply, ID: 7, RootID: 17, ParentID: &parentID,
	}
	commentID := uint64(17)
	cleaner := &previewCleanerStub{}
	repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil)
	repo.EXPECT().LoadSubject(gomock.Any(), record.Subject).Return(
		moderationrepo.SubjectSnapshot{
			Ref:      record.Subject,
			AuthorID: record.AuthorID, Content: "待审正文",
		}, nil,
	)
	repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), record.Subject).Return(
		moderationrepo.ReviewNotificationContext{
			ContentType: moderationrepo.SubjectArticleCommentReply, InteractionRecipientUserID: 99,
			CommentID: &commentID,
			RootSnapshot: &moderationrepo.NotificationSnapshot{
				Type: "article", ID: 3, Title: "测试文章",
			},
			QuoteSnapshot: &moderationrepo.NotificationSnapshot{
				Type: "comment", ID: 17, Excerpt: "被回复的评论",
			},
		}, nil,
	)
	repo.EXPECT().LoadRevisionPreviewKeys(gomock.Any(), record.RevisionID).
		Return([]string{"moderation/previews/a.jpg"}, nil)
	repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, cmd moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
			assert.Equal(t, record.AuthorID, cmd.AuthorID)
			assert.Equal(t, record.LockVersion, cmd.ExpectedLockVersion)
			require.NotNil(t, cmd.ExpectedPendingID)
			assert.Equal(t, record.RevisionID, *cmd.ExpectedPendingID)
			require.NotNil(t, cmd.Review)
			assert.Equal(t, moderationrepo.ReviewApproved, cmd.Review.Status)
			assert.Equal(t, "approved", cmd.Review.Decision)
			assert.Equal(t, moderationrepo.ExistingRevision(record.RevisionID), cmd.Materialize)
			require.NotNil(t, cmd.ProfileChange)
			assert.Equal(t, int64(1), cmd.ProfileChange.CleanApprovalDelta)
			assert.Equal(t, int64(-1), cmd.ProfileChange.ViolationScoreDelta)
			require.NotNil(t, cmd.Notification)
			assert.Equal(t, record.AuthorID, cmd.Notification.RecipientUserID)
			assert.Equal(t, "待审正文", cmd.Notification.ContentExcerpt)
			assert.Equal(t, "approved", cmd.Notification.Decision)
			assert.Equal(t, moderationrepo.SubjectArticleCommentReply, cmd.Notification.ContentType)
			require.NotNil(t, cmd.Notification.RootSnapshot)
			assert.Equal(t, "测试文章", cmd.Notification.RootSnapshot.Title)
			require.NotNil(t, cmd.InteractionNotification)
			assert.Equal(t, "reply_created", cmd.InteractionNotification.Type)
			assert.Equal(t, record.AuthorID, cmd.InteractionNotification.ActorUserID)
			assert.Equal(t, uint64(99), cmd.InteractionNotification.RecipientUserID)
			assert.Equal(t, "reply", cmd.InteractionNotification.SourceType)
			assert.Zero(t, cmd.InteractionNotification.SourceID)
			assert.Equal(t, "article", cmd.InteractionNotification.RootType)
			assert.Equal(t, uint64(3), cmd.InteractionNotification.RootID)
			require.NotNil(t, cmd.InteractionNotification.QuoteSnapshot)
			assert.Equal(t, uint64(17), cmd.InteractionNotification.QuoteSnapshot.ID)
			assert.Equal(t, "被回复的评论", cmd.InteractionNotification.QuoteSnapshot.Excerpt)
			return moderationrepo.AppliedTransition{Subject: record.Subject, ItemID: record.ItemID, LockVersion: 4}, nil
		})
	repo.EXPECT().LoadModerationProfile(gomock.Any(), record.AuthorID, serviceNow).Return(moderationrepo.ModerationProfile{
		UserID: record.AuthorID, TrustLevel: moderationrepo.TrustNew, TrustSource: moderationrepo.TrustSourceAuto,
		SanctionState: moderationrepo.SanctionActive, CleanApprovalStreak: 3,
		CreatedAt: serviceNow.AddDate(0, 0, -7), UpdatedAt: serviceNow,
	}, nil)
	repo.EXPECT().SetAutomaticTrust(gomock.Any(), moderationrepo.AutomaticTrustCommand{
		UserID: record.AuthorID, TrustLevel: moderationrepo.TrustNormal, UpdatedAt: serviceNow,
	}).Return(true, nil)
	service := moderation.NewReviewService(repo, &processorStub{}, cleaner, nil, config.ModerationConfig{
		Content: config.ModerationContentConfig{
			MomentMaxChars: 800, CommentMaxChars: 2000, GuestbookMaxChars: 2000, ReplyMaxChars: 2000,
		},
		Review: config.ModerationReviewConfig{QueueDefaultPageSize: 20, QueueMaxPageSize: 100, ReasonMaxChars: 1000},
		Governance: config.ModerationGovernanceConfig{
			NewToNormal:              config.ModerationPromotionConfig{MinAgeDays: 7, CleanApprovals: 3},
			RestrictedScoreThreshold: 6, RestrictedDuration: 168 * time.Hour,
			CleanApprovalScoreDecay: 1,
		},
	}, zap.NewNop(), func() time.Time { return serviceNow })

	got, err := service.Approve(context.Background(), moderation.ReviewCommand{
		ItemID: record.ItemID, RevisionID: record.RevisionID,
		ExpectedLockVersion: record.LockVersion, ReviewerID: 1,
	})

	require.NoError(t, err)
	assert.Equal(t, moderation.ReviewApproved, got.ReviewStatus)
	assert.Equal(t, uint64(4), got.LockVersion)
	assert.Equal(t, []string{"moderation/previews/a.jpg"}, cleaner.keys)
}

func TestReviewServiceApproveAbortsWhenRequiredInteractionContextFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	record := pendingReviewRecord()
	record.Subject = moderationrepo.SubjectRef{
		Type: moderationrepo.SubjectArticleComment,
		ID:   7, RootID: 3,
	}
	wantErr := errors.New("context unavailable")
	repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil)
	repo.EXPECT().LoadSubject(gomock.Any(), record.Subject).Return(
		moderationrepo.SubjectSnapshot{Ref: record.Subject, AuthorID: record.AuthorID, Content: "待审正文"}, nil,
	)
	repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), record.Subject).Return(
		moderationrepo.ReviewNotificationContext{}, wantErr,
	)
	service := newReviewService(repo, &processorStub{})

	_, err := service.Approve(context.Background(), moderation.ReviewCommand{
		ItemID: record.ItemID, RevisionID: record.RevisionID,
		ExpectedLockVersion: record.LockVersion, ReviewerID: 1,
	})

	require.ErrorIs(t, err, wantErr)
}

func TestReviewServiceCorrectSanitizesContentAndRecordsViolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	record := pendingReviewRecord()
	repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil)
	repo.EXPECT().LoadSubject(gomock.Any(), record.Subject).Return(
		moderationrepo.SubjectSnapshot{Ref: record.Subject, AuthorID: record.AuthorID, Content: record.PublishedContent}, nil,
	)
	repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), gomock.Any()).Return(
		moderationrepo.ReviewNotificationContext{
			ContentType: record.Subject.Type, InteractionRecipientUserID: 99,
			RootSnapshot: &moderationrepo.NotificationSnapshot{Type: "article", ID: record.Subject.RootID},
		}, nil,
	)
	repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, cmd moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
			require.NotNil(t, cmd.Review.PublishedContent)
			assert.Equal(t, "<p>修正正文</p>", *cmd.Review.PublishedContent)
			assert.Equal(t, "移除不当表述", *cmd.Review.Reason)
			assert.Equal(t, int64(1), cmd.ProfileChange.CorrectedDelta)
			assert.Equal(t, int64(1), cmd.ProfileChange.ViolationScoreDelta)
			assert.Equal(t, "corrected", cmd.Notification.Decision)
			require.NotNil(t, cmd.InteractionNotification)
			assert.Equal(t, "<p>修正正文</p>", cmd.InteractionNotification.ContentExcerpt)
			return moderationrepo.AppliedTransition{Subject: record.Subject, ItemID: record.ItemID, LockVersion: 4}, nil
		})
	processor := &processorStub{useOut: true, out: moderation.ProcessedContent{
		Published: "<p>修正正文</p>", PlainText: "修正正文",
	}}
	service := newReviewService(repo, processor)

	got, err := service.Correct(context.Background(), moderation.CorrectCommand{
		ReviewCommand: moderation.ReviewCommand{
			ItemID: record.ItemID, RevisionID: record.RevisionID,
			ExpectedLockVersion: record.LockVersion, ReviewerID: 1, Reason: "移除不当表述",
		},
		Content: "<p>修正正文</p><script>bad</script>",
	})

	require.NoError(t, err)
	assert.Equal(t, "<p>修正正文</p>", got.PublishedContent)
	assert.Equal(t, "<p>修正正文</p><script>bad</script>", processor.got)
}

func TestReviewServiceRejectRequiresReasonBeforeRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	service := newReviewService(repo, &processorStub{})

	_, err := service.Reject(context.Background(), moderation.ReviewCommand{
		ItemID: 10, RevisionID: 20, ExpectedLockVersion: 3, ReviewerID: 1,
	})

	require.ErrorIs(t, err, moderation.ErrInvalidRequest)
}

func TestReviewServiceListDefaultsToPendingQueue(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	repo.EXPECT().ListReviewRecords(gomock.Any(), moderationrepo.ReviewFilter{
		Page: 1, PageSize: 20, ReviewStatus: reviewStatusPtr(moderationrepo.ReviewPending),
	}).Return(moderationrepo.ReviewPage{Total: 1, Items: []moderationrepo.ReviewRecord{pendingReviewRecord()}}, nil)
	service := newReviewService(repo, &processorStub{})

	page, err := service.List(context.Background(), moderation.ListReviewCommand{})

	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, uint64(20), page.Items[0].RevisionID)
}

func TestReviewServiceListPassesPublicStateFilterAndMapsEmergencyHide(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	emergencyReason := "紧急下架劣迹内容"
	hiddenAt := serviceNow
	record := pendingReviewRecord()
	record.State.PublicState = moderationrepo.PublicEmergencyHidden
	record.State.EmergencyReason = &emergencyReason
	record.State.EmergencyHiddenAt = &hiddenAt
	emergencyState := moderationrepo.PublicEmergencyHidden
	repo.EXPECT().ListReviewRecords(gomock.Any(), moderationrepo.ReviewFilter{
		Page: 1, PageSize: 20, ReviewStatus: reviewStatusPtr(moderationrepo.ReviewApproved), PublicState: &emergencyState,
	}).Return(moderationrepo.ReviewPage{Total: 1, Items: []moderationrepo.ReviewRecord{record}}, nil)
	service := newReviewService(repo, &processorStub{})

	cmdState := moderation.PublicState(moderationrepo.PublicEmergencyHidden)
	page, err := service.List(context.Background(), moderation.ListReviewCommand{
		ReviewStatus: serviceReviewStatusPtr(moderation.ReviewApproved), PublicState: &cmdState,
	})

	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, moderation.PublicState(moderationrepo.PublicEmergencyHidden), page.Items[0].PublicState)
	require.NotNil(t, page.Items[0].EmergencyHideReason)
	assert.Equal(t, emergencyReason, *page.Items[0].EmergencyHideReason)
	require.NotNil(t, page.Items[0].EmergencyHiddenAt)
	assert.Equal(t, hiddenAt, *page.Items[0].EmergencyHiddenAt)
}

func TestReviewServiceDeletedItemReturnsTerminalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	record := pendingReviewRecord()
	deletedAt := serviceNow
	record.State = moderationrepo.ItemState{
		LifecycleState: moderationrepo.LifecycleDeleted, PublicState: moderationrepo.PublicHidden,
		DeletedAt: &deletedAt,
	}
	record.ReviewStatus = moderationrepo.ReviewSuperseded
	repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil)
	service := newReviewService(repo, &processorStub{})

	_, err := service.Approve(context.Background(), moderation.ReviewCommand{
		ItemID: record.ItemID, RevisionID: record.RevisionID,
		ExpectedLockVersion: record.LockVersion, ReviewerID: 1,
	})

	require.ErrorIs(t, err, moderation.ErrAlreadyDeleted)
}

func newReviewService(repo moderationrepo.Repository, processor moderation.ContentProcessor, cleaners ...moderation.PreviewCleaner) moderation.ReviewService {
	cfg := config.ModerationConfig{
		Content: config.ModerationContentConfig{
			MomentMaxChars: 800, CommentMaxChars: 2000, GuestbookMaxChars: 2000, ReplyMaxChars: 2000,
		},
		Review: config.ModerationReviewConfig{
			QueueDefaultPageSize: 20, QueueMaxPageSize: 100, ReasonMaxChars: 1000,
		},
		Governance: config.ModerationGovernanceConfig{
			CleanApprovalScoreDecay: 1,
			ViolationWeights:        config.ModerationViolationWeightsConfig{Corrected: 1, Rejected: 3},
		},
	}
	var cleaner moderation.PreviewCleaner
	if len(cleaners) > 0 {
		cleaner = cleaners[0]
	}
	return moderation.NewReviewService(repo, processor, cleaner, nil, cfg, zap.NewNop(), func() time.Time { return serviceNow })
}

func pendingReviewRecord() moderationrepo.ReviewRecord {
	return moderationrepo.ReviewRecord{
		ItemID: 10, Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, LockVersion: 3,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
			Materialized: moderationrepo.ExistingRevision(20), Pending: moderationrepo.ExistingRevision(20),
		},
		RevisionID: 20, RevisionVersion: 1, SubmittedContent: "用户原文", PublishedContent: "待审正文",
		RiskLevel: moderationrepo.RiskLow, PolicyAction: moderationrepo.ActionPostReview,
		ReviewStatus: moderationrepo.ReviewPending, CreatedAt: serviceNow,
	}
}

func reviewStatusPtr(status moderationrepo.ReviewStatus) *moderationrepo.ReviewStatus {
	return &status
}

func serviceReviewStatusPtr(status moderation.ReviewStatus) *moderation.ReviewStatus {
	return &status
}

func TestReviewServiceListIncludesAllStatusesWhenRequested(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	repo.EXPECT().ListReviewRecords(gomock.Any(), moderationrepo.ReviewFilter{
		Page: 1, PageSize: 20,
	}).Return(moderationrepo.ReviewPage{Total: 2, Items: []moderationrepo.ReviewRecord{
		pendingReviewRecord(),
		func() moderationrepo.ReviewRecord {
			record := pendingReviewRecord()
			record.ReviewStatus = moderationrepo.ReviewRejected
			return record
		}(),
	}}, nil)
	service := newReviewService(repo, &processorStub{})

	page, err := service.List(context.Background(), moderation.ListReviewCommand{IncludeAllReviewStatuses: true})

	require.NoError(t, err)
	assert.Equal(t, int64(2), page.Total)
	require.Len(t, page.Items, 2)
}
