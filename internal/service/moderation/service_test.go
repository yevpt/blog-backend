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
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

var serviceNow = time.Date(2026, time.June, 28, 12, 0, 0, 0, time.UTC)

type processorStub struct {
	calls  int
	got    string
	out    moderation.ProcessedContent
	err    error
	useOut bool
}

func (p *processorStub) Process(raw string, _ int) (moderation.ProcessedContent, error) {
	p.calls++
	p.got = raw
	if !p.useOut && p.out.Published == "" && p.out.PlainText == "" {
		return moderation.ProcessedContent{Published: "<p>safe</p>", PlainText: "safe"}, p.err
	}
	return p.out, p.err
}

type classifierStub struct {
	calls int
	out   moderation.Classification
}

func (c *classifierStub) Classify(moderation.ProcessedContent) moderation.Classification {
	c.calls++
	return c.out
}

func (c *classifierStub) ReplaceSnapshot(*ruleindex.Snapshot) error { return nil }

type deciderStub struct {
	calls  int
	action moderation.PolicyAction
	err    error
	input  moderation.PolicyInput
}

func (d *deciderStub) Decide(input moderation.PolicyInput) (moderation.PolicyAction, error) {
	d.calls++
	d.input = input
	return d.action, d.err
}

type mediaServiceStub struct {
	set moderationmedia.PreparedSet
	err error
}

func (s *mediaServiceStub) Prepare(context.Context, uint64, []string) (moderationmedia.PreparedSet, error) {
	return s.set, s.err
}

func newApplicationService(
	repo moderationrepo.Repository,
	processor moderation.ContentProcessor,
	classifier moderation.Classifier,
	decider moderation.PolicyDecider,
	logger *zap.Logger,
	media ...moderation.MediaService,
) moderation.Service {
	cfg := config.ModerationConfig{
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
	var mediaService moderation.MediaService
	if len(media) > 0 {
		mediaService = media[0]
	}
	return moderation.NewService(repo, processor, classifier, decider, mediaService, nil, cfg, logger, func() time.Time {
		return serviceNow
	})
}

func submitCommand() moderation.SubmitCommand {
	return moderation.SubmitCommand{
		ActorID:        7,
		Subject:        moderation.SubjectRef{Type: moderation.SubjectArticleComment, RootID: 11},
		Content:        "<p>raw</p>",
		IdempotencyKey: "request-1",
	}
}

func normalPolicyContext() moderationrepo.PolicyContext {
	return moderationrepo.PolicyContext{
		TrustLevel: moderationrepo.TrustNormal, SanctionState: moderationrepo.SanctionActive,
		PublishingMode: moderationrepo.PublishingOpen,
	}
}

func TestServiceSubmitLowPostReview(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{out: moderation.Classification{
		Risk: moderation.RiskLow, RuleMatchIDs: []uint64{3}, RuleMatchesTruncated: true, RulesetVersion: 9,
	}}
	decider := &deciderStub{action: moderation.ActionPostReview}
	cmd := submitCommand()

	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				assert.True(t, persisted.CreateSubject)
				assert.Nil(t, persisted.InteractionNotification)
				require.NotNil(t, persisted.Revision)
				assert.Equal(t, moderationrepo.RiskLow, persisted.Revision.RiskLevel)
				assert.Equal(t, moderationrepo.ActionPostReview, persisted.Revision.PolicyAction)
				assert.Equal(t, moderationrepo.ReviewPending, persisted.Revision.ReviewStatus)
				assert.Equal(t, uint64(9), persisted.Revision.RulesetVersion)
				assert.Equal(t, []uint64{3}, persisted.Revision.RuleMatchIDs)
				assert.True(t, persisted.Revision.RuleMatchesTruncated)
				assert.True(t, persisted.Next.Pending.IsNew)
				assert.True(t, persisted.Next.Materialized.IsNew)
				assert.False(t, persisted.Next.Approved.IsNew)
				return moderationrepo.AppliedTransition{
					Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 41, RootID: 11},
					ItemID:  51, RevisionID: 61, RevisionVersion: 1, LockVersion: 2,
				}, nil
			},
		),
	)

	got, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Submit(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, "发布成功，内容会被审核。", got.Message)
	assert.Equal(t, moderation.ActionPostReview, got.Action)
	assert.Equal(t, moderation.RiskLow, got.RiskLevel)
	assert.Equal(t, "<p>safe</p>", got.Content)
	assert.True(t, got.HasPendingRevision)
	assert.False(t, got.CanInteract)
	assert.Equal(t, uint64(41), got.Subject.ID)
}

func TestServiceSubmitMomentCarriesBusinessOptionsToTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := moderation.SubmitCommand{
		ActorID: 7, AuthorID: 99, IsAdmin: true,
		Subject: moderation.SubjectRef{Type: moderation.SubjectMoment}, Content: "隐藏碎语",
		IdempotencyKey: "moment-options", MomentOptions: &moderation.MomentOptions{Status: 0, CommentStatus: 0},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				assert.Equal(t, uint64(99), persisted.AuthorID)
				require.NotNil(t, persisted.Log)
				require.NotNil(t, persisted.Log.ActorUserID)
				assert.Equal(t, uint64(7), *persisted.Log.ActorUserID)
				require.NotNil(t, persisted.MomentOptions)
				assert.Equal(t, uint8(0), persisted.MomentOptions.Status)
				assert.Equal(t, uint8(0), persisted.MomentOptions.CommentStatus)
				return moderationrepo.AppliedTransition{
					Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectMoment, ID: 41},
					ItemID:  51, RevisionID: 61, RevisionVersion: 1, LockVersion: 2,
				}, nil
			},
		),
	)

	_, err := newApplicationService(
		repo, &processorStub{}, &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}},
		&deciderStub{action: moderation.ActionPostReview}, zap.NewNop(),
	).Submit(context.Background(), cmd)

	require.NoError(t, err)
}

func TestServiceSubmitMediumHidesBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{out: moderation.Classification{Risk: moderation.RiskMedium, RulesetVersion: 4}}
	decider := &deciderStub{action: moderation.ActionPreReview}
	cmd := submitCommand()

	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				assert.True(t, persisted.Next.Pending.IsNew)
				assert.False(t, persisted.Next.Materialized.IsNew)
				assert.Equal(t, moderationrepo.PublicPlaceholder, persisted.Next.PublicState)
				return moderationrepo.AppliedTransition{
					Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 42, RootID: 11},
					ItemID:  52, RevisionID: 62, RevisionVersion: 1, LockVersion: 2,
				}, nil
			},
		),
	)

	got, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Submit(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, "内容已提交，等待人工审核。", got.Message)
	assert.Empty(t, got.Content)
	assert.Equal(t, moderation.PublicPlaceholder, got.PublicState)
	assert.False(t, got.CanInteract)
}

func TestServiceSubmitHighRiskStillRejectsWhenAuditFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{out: moderation.Classification{
		Risk: moderation.RiskHigh, RuleMatchIDs: []uint64{99}, RuleMatchesTruncated: true, RulesetVersion: 8,
	}}
	decider := &deciderStub{action: moderation.ActionBlock}
	cmd := submitCommand()
	core, observed := observer.New(zap.ErrorLevel)

	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().RecordBlockedAttempt(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, attempt moderationrepo.BlockedAttempt) (moderationrepo.StoredResult, error) {
				assert.Equal(t, []uint64{99}, attempt.RuleMatchIDs)
				assert.True(t, attempt.RuleMatchesTruncated)
				assert.Equal(t, uint64(8), attempt.RulesetVersion)
				return moderationrepo.StoredResult{}, errors.New("audit down")
			},
		),
	)

	_, err := newApplicationService(repo, processor, classifier, decider, zap.New(core)).Submit(context.Background(), cmd)
	require.ErrorIs(t, err, moderation.ErrContentRiskRejected)
	assert.Contains(t, err.Error(), "内容存在较高风险")
	require.Len(t, observed.All(), 1)
	assert.Equal(t, "记录高风险审核尝试失败", observed.All()[0].Message)
}

func TestServicePersistsPreparedImagesAndSelectsUnapprovedImagePolicy(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{useOut: true, out: moderation.ProcessedContent{Published: "raw-image", PlainText: "safe"}}
	classifier := &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}}
	decider := &deciderStub{action: moderation.ActionPostReview}
	media := &mediaServiceStub{set: moderationmedia.PreparedSet{Images: []moderationmedia.PreparedImage{{
		Fingerprint: moderationmedia.Fingerprint{SHA256: "sha", MD5: "md5", Size: 10},
		ObjectKey:   "moments/7/a.jpg", MediaType: "image/jpeg",
		PreviewObjectKey: "moderation/previews/a.jpg",
	}}, Replacements: map[string]string{"raw-image": "moments/7/a.jpg"}}}
	cmd := submitCommand()
	cmd.ImageKeys = []string{"moments/7/a.jpg"}
	repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil)
	repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil)
	repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
			require.Len(t, persisted.Revision.Images, 1)
			assert.Equal(t, "sha", persisted.Revision.Images[0].SHA256)
			assert.Equal(t, uint(1), persisted.Revision.Images[0].Seq)
			assert.Equal(t, "moments/7/a.jpg", persisted.Revision.PublishedContent)
			return moderationrepo.AppliedTransition{
				Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 88, RootID: 11},
				ItemID:  10, RevisionID: 20, RevisionVersion: 1, LockVersion: 2,
			}, nil
		})

	result, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop(), media).Submit(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, decider.input.HasUnapprovedImage)
	assert.Equal(t, "moderation/previews/a.jpg", result.Content)
	require.Len(t, result.Images, 1)
	assert.Equal(t, "moments/7/a.jpg", result.Images[0].DisplayObjectKey)
	assert.True(t, result.Images[0].Approved)
}

func TestServiceIdempotentRetryReturnsStoredResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{}
	decider := &deciderStub{}
	cmd := submitCommand()
	repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).
		Return(&moderationrepo.StoredResult{
			Kind:    moderationrepo.ResultRevision,
			Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 88, RootID: 11},
			ItemID:  10, RevisionID: 20, RevisionVersion: 2, LockVersion: 3,
			RiskLevel: moderationrepo.RiskLow, PolicyAction: moderationrepo.ActionPostReview,
			ReviewStatus: moderationrepo.ReviewPending, PublicState: moderationrepo.PublicVisible,
			Content: "<p>first</p>",
		}, nil)

	got, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Submit(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, "<p>first</p>", got.Content)
	assert.Equal(t, uint64(20), got.RevisionID)
	assert.Equal(t, "发布成功，内容会被审核。", got.Message)
	assert.Zero(t, processor.calls)
}

func TestServiceIdempotentMediumEditKeepsVisibleVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := moderation.EditCommand{
		ActorID: 7, Subject: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 88},
		Content: "中风险编辑", IdempotencyKey: "medium-edit-replay",
	}
	repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(&moderationrepo.StoredResult{
		Kind: moderationrepo.ResultRevision, Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 88, RootID: 11},
		ItemID: 10, RevisionID: 20, RiskLevel: moderationrepo.RiskMedium,
		PolicyAction: moderationrepo.ActionPreReview, ReviewStatus: moderationrepo.ReviewPending,
		PublicState: moderationrepo.PublicVisible, Content: "中风险编辑", VisibleContent: "最后通过正文",
	}, nil)

	got, err := newApplicationService(repo, &processorStub{}, &classifierStub{}, &deciderStub{}, zap.NewNop()).Edit(context.Background(), cmd)

	require.NoError(t, err)
	assert.Equal(t, "最后通过正文", got.Content)
	require.NotNil(t, got.PendingContent)
	assert.Equal(t, "中风险编辑", *got.PendingContent)
	assert.False(t, got.CanInteract)
}

func TestServiceIdempotentApprovedPostReviewCanInteract(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := submitCommand()
	repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(&moderationrepo.StoredResult{
		Kind: moderationrepo.ResultRevision, Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 88, RootID: 11},
		ItemID: 10, RevisionID: 20, RiskLevel: moderationrepo.RiskLow,
		PolicyAction: moderationrepo.ActionPostReview, ReviewStatus: moderationrepo.ReviewApproved,
		PublicState: moderationrepo.PublicVisible, Content: "已通过正文", VisibleContent: "已通过正文",
	}, nil)

	got, err := newApplicationService(repo, &processorStub{}, &classifierStub{}, &deciderStub{}, zap.NewNop()).Submit(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, got.CanInteract)
	assert.False(t, got.HasPendingRevision)
}

func TestServiceAdminSubmitAutoApproves(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{out: moderation.Classification{Risk: moderation.RiskHigh, RulesetVersion: 3}}
	decider := &deciderStub{action: moderation.ActionAutoApprove}
	cmd := submitCommand()
	cmd.IsAdmin = true

	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(
			moderationrepo.ReviewNotificationContext{}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				require.NotNil(t, persisted.Revision)
				assert.Equal(t, moderationrepo.ReviewApproved, persisted.Revision.ReviewStatus)
				assert.True(t, persisted.Next.Approved.IsNew)
				assert.True(t, persisted.Next.Materialized.IsNew)
				assert.False(t, persisted.Next.Pending.IsNew)
				assert.Zero(t, persisted.Next.Pending.ID)
				return moderationrepo.AppliedTransition{
					Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 43, RootID: 11},
					ItemID:  53, RevisionID: 63, RevisionVersion: 1, LockVersion: 2,
				}, nil
			},
		),
	)

	got, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Submit(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, moderation.ActionAutoApprove, got.Action)
	assert.False(t, got.HasPendingRevision)
	assert.True(t, got.CanInteract)
}

func TestServiceAutoApproveFirstPublicationCreatesInteractionNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := submitCommand()
	notifCtx := moderationrepo.ReviewNotificationContext{
		ContentType: moderationrepo.SubjectArticleComment, InteractionRecipientUserID: 91,
		RootSnapshot: &moderationrepo.NotificationSnapshot{Type: "article", ID: cmd.Subject.RootID},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(notifCtx, nil),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				require.NotNil(t, persisted.InteractionNotification)
				intent := persisted.InteractionNotification
				assert.Equal(t, "comment_created", intent.Type)
				assert.Equal(t, cmd.ActorID, intent.ActorUserID)
				assert.Equal(t, uint64(91), intent.RecipientUserID)
				assert.Equal(t, "<p>safe</p>", intent.ContentExcerpt)
				assert.Same(t, notifCtx.RootSnapshot, intent.RootSnapshot)
				return moderationrepo.AppliedTransition{
					Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 41, RootID: 11},
					ItemID:  51, RevisionID: 61, RevisionVersion: 1, LockVersion: 2,
				}, nil
			},
		),
	)

	_, err := newApplicationService(
		repo, &processorStub{}, &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}},
		&deciderStub{action: moderation.ActionAutoApprove}, zap.NewNop(),
	).Submit(context.Background(), cmd)

	require.NoError(t, err)
}

func TestServiceAutoApprovePendingWithoutApprovedCreatesInteractionNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := moderation.EditCommand{
		ActorID: 7, Subject: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 41},
		Content: "approved edit", IdempotencyKey: "approve-pending",
	}
	canonicalRef := moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 41, RootID: 11}
	item := moderationrepo.ItemStateRecord{
		ItemID: 20, AuthorID: 7, LockVersion: 4,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicPlaceholder,
			Pending: moderationrepo.ExistingRevision(10),
		},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(item, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(
			moderationrepo.SubjectSnapshot{Ref: canonicalRef, AuthorID: 7}, nil,
		),
		repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), canonicalRef).Return(
			moderationrepo.ReviewNotificationContext{InteractionRecipientUserID: 91}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				require.NotNil(t, persisted.InteractionNotification)
				assert.Equal(t, uint64(91), persisted.InteractionNotification.RecipientUserID)
				return moderationrepo.AppliedTransition{
					Subject: canonicalRef, ItemID: 20, RevisionID: 30, RevisionVersion: 2, LockVersion: 5,
				}, nil
			},
		),
	)

	_, err := newApplicationService(
		repo, &processorStub{}, &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}},
		&deciderStub{action: moderation.ActionAutoApprove}, zap.NewNop(),
	).Edit(context.Background(), cmd)

	require.NoError(t, err)
}

func TestServiceAutoApproveVisibleEditDoesNotCreateInteractionNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := moderation.EditCommand{
		ActorID: 7, Subject: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 41},
		Content: "visible edit", IdempotencyKey: "edit-visible-approved",
	}
	canonicalRef := moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 41, RootID: 11}
	item := moderationrepo.ItemStateRecord{
		ItemID: 20, AuthorID: 7, LockVersion: 4,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
			Materialized: moderationrepo.ExistingRevision(10), Approved: moderationrepo.ExistingRevision(10),
		},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(item, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(
			moderationrepo.SubjectSnapshot{Ref: canonicalRef, AuthorID: 7, Content: "old"}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				assert.Nil(t, persisted.InteractionNotification)
				return moderationrepo.AppliedTransition{
					Subject: canonicalRef, ItemID: 20, RevisionID: 30, RevisionVersion: 2, LockVersion: 5,
				}, nil
			},
		),
	)

	_, err := newApplicationService(
		repo, &processorStub{}, &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}},
		&deciderStub{action: moderation.ActionAutoApprove}, zap.NewNop(),
	).Edit(context.Background(), cmd)

	require.NoError(t, err)
}

func TestServiceAutoApproveNotificationContextFailureAbortsPublication(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := submitCommand()
	wantErr := errors.New("notification context unavailable")
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadReviewNotificationContext(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(
			moderationrepo.ReviewNotificationContext{}, wantErr,
		),
	)

	_, err := newApplicationService(
		repo, &processorStub{}, &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}},
		&deciderStub{action: moderation.ActionAutoApprove}, zap.NewNop(),
	).Submit(context.Background(), cmd)

	require.ErrorIs(t, err, wantErr)
}

func TestServiceAutoApproveNotificationIdempotencyReplayDoesNotWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := submitCommand()
	repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(&moderationrepo.StoredResult{
		Kind:     moderationrepo.ResultRevision,
		Subject:  moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 41, RootID: 11},
		AuthorID: 7, ItemID: 51, RevisionID: 61, RevisionVersion: 1, LockVersion: 2,
		RiskLevel: moderationrepo.RiskLow, PolicyAction: moderationrepo.ActionAutoApprove,
		ReviewStatus: moderationrepo.ReviewApproved, PublicState: moderationrepo.PublicVisible,
		Content: "<p>safe</p>", VisibleContent: "<p>safe</p>",
	}, nil)

	_, err := newApplicationService(
		repo, &processorStub{}, &classifierStub{}, &deciderStub{}, zap.NewNop(),
	).Submit(context.Background(), cmd)

	require.NoError(t, err)
}

func TestServiceEditLowAndMediumBuildExpectedTransitions(t *testing.T) {
	tests := []struct {
		name              string
		risk              moderation.RiskLevel
		action            moderation.PolicyAction
		wantMaterialized  bool
		wantPublicContent string
	}{
		{"low", moderation.RiskLow, moderation.ActionPostReview, true, "<p>safe</p>"},
		{"medium", moderation.RiskMedium, moderation.ActionPreReview, false, "旧正文"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := repositorymock.NewMockRepository(ctrl)
			processor := &processorStub{}
			classifier := &classifierStub{out: moderation.Classification{Risk: tt.risk, RulesetVersion: 3}}
			decider := &deciderStub{action: tt.action}
			cmd := moderation.EditCommand{
				ActorID: 7, Subject: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 41},
				Content: "<p>edit</p>", IdempotencyKey: "edit-1",
			}
			canonicalRef := moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 41, RootID: 11}
			approvedID := uint64(10)
			item := moderationrepo.ItemStateRecord{
				ItemID: 20, AuthorID: 7, LockVersion: 4,
				State: moderationrepo.ItemState{
					LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
					Materialized: moderationrepo.ExistingRevision(approvedID),
					Approved:     moderationrepo.ExistingRevision(approvedID),
				},
			}
			gomock.InOrder(
				repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
				repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
				repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(item, nil),
				repo.EXPECT().LoadSubject(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(
					moderationrepo.SubjectSnapshot{Ref: canonicalRef, AuthorID: 7, Content: "旧正文"}, nil,
				),
				repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
						assert.Equal(t, uint64(4), persisted.ExpectedLockVersion)
						assert.Equal(t, tt.wantMaterialized, persisted.Next.Materialized.IsNew)
						return moderationrepo.AppliedTransition{
							Subject: canonicalRef, ItemID: 20,
							RevisionID: 30, RevisionVersion: 2, LockVersion: 5,
						}, nil
					},
				),
			)

			got, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Edit(context.Background(), cmd)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPublicContent, got.Content)
		})
	}
}

func TestServiceHighRiskEditKeepsExistingVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{out: moderation.Classification{Risk: moderation.RiskHigh, RulesetVersion: 5}}
	decider := &deciderStub{action: moderation.ActionBlock}
	cmd := moderation.EditCommand{
		ActorID: 7, Subject: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 41, RootID: 11},
		Content: "risk", IdempotencyKey: "edit-risk",
	}
	item := moderationrepo.ItemStateRecord{
		ItemID: 20, AuthorID: 7, LockVersion: 4,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
			Materialized: moderationrepo.ExistingRevision(10), Approved: moderationrepo.ExistingRevision(10),
		},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(item, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(
			moderationrepo.SubjectSnapshot{Ref: moderationrepo.SubjectRef(cmd.Subject), AuthorID: 7, Content: "旧正文"}, nil,
		),
		repo.EXPECT().RecordBlockedAttempt(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, attempt moderationrepo.BlockedAttempt) (moderationrepo.StoredResult, error) {
				require.NotNil(t, attempt.ItemID)
				assert.Equal(t, uint64(20), *attempt.ItemID)
				require.NotNil(t, attempt.ProfileChange)
				assert.Equal(t, int64(1), attempt.ProfileChange.HighRiskDelta)
				assert.Equal(t, int64(5), attempt.ProfileChange.ViolationScoreDelta)
				return moderationrepo.StoredResult{Kind: moderationrepo.ResultBlocked}, nil
			},
		),
	)

	_, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Edit(context.Background(), cmd)
	require.ErrorIs(t, err, moderation.ErrContentRiskRejected)
	assert.Contains(t, err.Error(), "原内容不受影响")
}

func TestServiceReconcilesAutomaticRestrictionBeforePublishingPolicy(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	cmd := submitCommand()
	violationAt := serviceNow.Add(-time.Hour)
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadModerationProfile(gomock.Any(), cmd.ActorID, serviceNow).Return(moderationrepo.ModerationProfile{
			UserID: cmd.ActorID, TrustLevel: moderationrepo.TrustNormal, TrustSource: moderationrepo.TrustSourceAuto,
			SanctionState: moderationrepo.SanctionActive, ViolationScore: 6, LastViolationAt: &violationAt,
			CreatedAt: serviceNow.AddDate(0, -2, 0), UpdatedAt: serviceNow,
		}, nil),
		repo.EXPECT().SetAutomaticTrust(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, update moderationrepo.AutomaticTrustCommand) (bool, error) {
				assert.Equal(t, moderationrepo.TrustRestricted, update.TrustLevel)
				return true, nil
			}),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(moderationrepo.PolicyContext{
			TrustLevel: moderationrepo.TrustRestricted, SanctionState: moderationrepo.SanctionActive,
			PublishingMode: moderationrepo.PublishingClosed,
		}, nil),
	)
	cfg := config.ModerationConfig{
		Enabled: true,
		Governance: config.ModerationGovernanceConfig{
			RestrictedScoreThreshold: 6, RestrictedDuration: 168 * time.Hour,
		},
	}
	service := moderation.NewService(repo, &processorStub{}, &classifierStub{}, &deciderStub{}, nil, nil, cfg, zap.NewNop(), func() time.Time { return serviceNow })

	_, err := service.Submit(context.Background(), cmd)

	require.ErrorIs(t, err, moderation.ErrPublishingForbidden)
}

func TestServiceHighRiskRaceReturnsFirstRevisionResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{out: moderation.Classification{Risk: moderation.RiskHigh, RulesetVersion: 5}}
	decider := &deciderStub{action: moderation.ActionBlock}
	cmd := submitCommand()
	first := &moderationrepo.StoredResult{
		Kind:    moderationrepo.ResultRevision,
		Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 44},
		ItemID:  22, RevisionID: 33, RiskLevel: moderationrepo.RiskLow,
		PolicyAction: moderationrepo.ActionPostReview, ReviewStatus: moderationrepo.ReviewPending,
		PublicState: moderationrepo.PublicVisible, Content: "首次正文",
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().RecordBlockedAttempt(gomock.Any(), gomock.Any()).
			Return(moderationrepo.StoredResult{}, moderationrepo.ErrIdempotencyDomainConflict),
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(first, nil),
	)

	got, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Submit(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, uint64(33), got.RevisionID)
	assert.Equal(t, "首次正文", got.Content)
	assert.Equal(t, uint64(11), got.Subject.RootID)
}

func TestServiceSanctionBlocksBeforeContentProcessing(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{}
	decider := &deciderStub{}
	cmd := submitCommand()
	repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil)
	repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(moderationrepo.PolicyContext{
		TrustLevel: moderationrepo.TrustNormal, SanctionState: moderationrepo.SanctionMuted,
		PublishingMode: moderationrepo.PublishingOpen,
	}, nil)

	_, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Submit(context.Background(), cmd)
	require.ErrorIs(t, err, moderation.ErrPublishingForbidden)
	assert.Zero(t, processor.calls)
	assert.Zero(t, classifier.calls)
	assert.Zero(t, decider.calls)
}

func TestServiceRejectsContentRemovedBySanitizer(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{out: moderation.ProcessedContent{}, useOut: true}
	classifier := &classifierStub{}
	decider := &deciderStub{}
	cmd := submitCommand()
	cmd.Content = "<script>alert(1)</script>"
	repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil)
	repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil)

	_, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Submit(context.Background(), cmd)

	require.ErrorIs(t, err, moderation.ErrInvalidRequest)
	assert.Zero(t, classifier.calls)
	assert.Zero(t, decider.calls)
}

func TestServiceAdminEditUsesContentAuthorForPersistence(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	processor := &processorStub{}
	classifier := &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow, RulesetVersion: 5}}
	decider := &deciderStub{action: moderation.ActionAutoApprove}
	cmd := moderation.EditCommand{
		ActorID: 99, IsAdmin: true,
		Subject: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 41, RootID: 11},
		Content: "admin edit", IdempotencyKey: "admin-edit",
	}
	item := moderationrepo.ItemStateRecord{
		ItemID: 20, AuthorID: 7, LockVersion: 4,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
			Materialized: moderationrepo.ExistingRevision(10), Approved: moderationrepo.ExistingRevision(10),
		},
	}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(item, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), moderationrepo.SubjectRef(cmd.Subject)).Return(
			moderationrepo.SubjectSnapshot{Ref: moderationrepo.SubjectRef(cmd.Subject), AuthorID: 7, Content: "旧正文"}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, persisted moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				assert.Equal(t, uint64(7), persisted.AuthorID)
				require.NotNil(t, persisted.Revision)
				assert.Equal(t, uint64(99), persisted.Revision.SubmitterID)
				require.NotNil(t, persisted.Revision.ReviewerID)
				assert.Equal(t, uint64(99), *persisted.Revision.ReviewerID)
				return moderationrepo.AppliedTransition{Subject: moderationrepo.SubjectRef(cmd.Subject), ItemID: 20, RevisionID: 30}, nil
			},
		),
	)

	_, err := newApplicationService(repo, processor, classifier, decider, zap.NewNop()).Edit(context.Background(), cmd)
	require.NoError(t, err)
}

func TestServiceAssertCanInteractUsesDerivedState(t *testing.T) {
	tests := []struct {
		name    string
		state   moderationrepo.ItemState
		wantErr error
	}{
		{
			name: "approved visible",
			state: moderationrepo.ItemState{
				LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
				Materialized: moderationrepo.ExistingRevision(10), Approved: moderationrepo.ExistingRevision(10),
			},
		},
		{
			name: "pending",
			state: moderationrepo.ItemState{
				LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
				Materialized: moderationrepo.ExistingRevision(11), Approved: moderationrepo.ExistingRevision(10),
				Pending: moderationrepo.ExistingRevision(11),
			},
			wantErr: moderation.ErrInteractionNotAllowed,
		},
		{
			name: "placeholder",
			state: moderationrepo.ItemState{
				LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicPlaceholder,
				Pending: moderationrepo.ExistingRevision(11),
			},
			wantErr: moderation.ErrInteractionNotAllowed,
		},
		{
			name: "hidden",
			state: moderationrepo.ItemState{
				LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicHidden,
				Materialized: moderationrepo.ExistingRevision(10), Approved: moderationrepo.ExistingRevision(10),
			},
			wantErr: moderation.ErrInteractionNotAllowed,
		},
		{
			name: "emergency hidden",
			state: moderationrepo.ItemState{
				LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicEmergencyHidden,
				Materialized: moderationrepo.ExistingRevision(10), Approved: moderationrepo.ExistingRevision(10),
			},
			wantErr: moderation.ErrInteractionNotAllowed,
		},
		{
			name: "deleted",
			state: moderationrepo.ItemState{
				LifecycleState: moderationrepo.LifecycleDeleted, PublicState: moderationrepo.PublicHidden,
				Approved: moderationrepo.ExistingRevision(10),
			},
			wantErr: moderation.ErrInteractionNotAllowed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := repositorymock.NewMockRepository(ctrl)
			ref := moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 4, RootID: 2}
			repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(ref)).
				Return(moderationrepo.ItemStateRecord{ItemID: 8, AuthorID: 7, LockVersion: 3, State: tt.state}, nil)
			if tt.wantErr == nil {
				repo.EXPECT().LoadSubject(gomock.Any(), moderationrepo.SubjectRef(ref)).
					Return(moderationrepo.SubjectSnapshot{Ref: moderationrepo.SubjectRef(ref), AuthorID: 7, Content: "正文"}, nil)
			}

			err := newApplicationService(repo, &processorStub{}, &classifierStub{}, &deciderStub{}, zap.NewNop()).
				AssertCanInteract(context.Background(), ref)

			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestServiceDeleteBuildsTerminalTransition(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	ref := moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 4}
	canonicalRef := moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 4, RootID: 2}
	item := moderationrepo.ItemStateRecord{
		ItemID: 8, AuthorID: 7, LockVersion: 3,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
			Materialized: moderationrepo.ExistingRevision(10), Approved: moderationrepo.ExistingRevision(10),
		},
	}
	gomock.InOrder(
		repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(ref)).Return(item, nil),
		repo.EXPECT().LoadSubject(gomock.Any(), moderationrepo.SubjectRef(ref)).Return(
			moderationrepo.SubjectSnapshot{Ref: canonicalRef, AuthorID: 7, Content: "正文"}, nil,
		),
		repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, command moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
				assert.Equal(t, canonicalRef, command.Subject)
				assert.Equal(t, moderationrepo.LifecycleDeleted, command.Next.LifecycleState)
				assert.Equal(t, moderationrepo.PublicHidden, command.Next.PublicState)
				assert.True(t, command.DeleteSubject)
				assert.Equal(t, uint64(3), command.ExpectedLockVersion)
				require.NotNil(t, command.Log)
				assert.Equal(t, moderationrepo.EventDelete, command.Log.Action)
				return moderationrepo.AppliedTransition{}, nil
			},
		),
	)

	err := newApplicationService(repo, &processorStub{}, &classifierStub{}, &deciderStub{}, zap.NewNop()).
		Delete(context.Background(), moderation.DeleteCommand{ActorID: 7, Subject: ref})

	require.NoError(t, err)
}

func TestServiceDeleteIsIdempotentAfterTerminalState(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	ref := moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 4}
	repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(ref)).Return(moderationrepo.ItemStateRecord{
		ItemID: 8, AuthorID: 7, LockVersion: 4,
		State: moderationrepo.ItemState{
			LifecycleState: moderationrepo.LifecycleDeleted, PublicState: moderationrepo.PublicHidden,
			DeletedAt: &serviceNow,
		},
	}, nil)

	err := newApplicationService(repo, &processorStub{}, &classifierStub{}, &deciderStub{}, zap.NewNop()).
		Delete(context.Background(), moderation.DeleteCommand{ActorID: 7, Subject: ref})

	require.NoError(t, err)
}

func TestServiceEditCannotReviveDeletedContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	ref := moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 4}
	cmd := moderation.EditCommand{ActorID: 7, Subject: ref, Content: "编辑", IdempotencyKey: "deleted-edit"}
	gomock.InOrder(
		repo.EXPECT().FindResultByIdempotencyKey(gomock.Any(), cmd.ActorID, cmd.IdempotencyKey).Return(nil, nil),
		repo.EXPECT().LoadPolicyContext(gomock.Any(), cmd.ActorID).Return(normalPolicyContext(), nil),
		repo.EXPECT().LoadItemState(gomock.Any(), moderationrepo.SubjectRef(ref)).Return(moderationrepo.ItemStateRecord{
			ItemID: 8, AuthorID: 7, LockVersion: 4,
			State: moderationrepo.ItemState{LifecycleState: moderationrepo.LifecycleDeleted, PublicState: moderationrepo.PublicHidden},
		}, nil),
	)

	_, err := newApplicationService(repo, &processorStub{}, &classifierStub{out: moderation.Classification{Risk: moderation.RiskLow}}, &deciderStub{action: moderation.ActionPostReview}, zap.NewNop()).
		Edit(context.Background(), cmd)

	assert.ErrorIs(t, err, moderation.ErrAlreadyDeleted)
}

func TestServiceLoadViewsDelegatesStableKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	ref := moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 4, RootID: 2}
	viewer := moderation.Viewer{Role: moderationrepo.ViewerPublic}
	want := map[moderation.SubjectKey]moderation.View{
		ref.Key(): {PublicState: moderationrepo.PublicVisible, VisibleContent: "正文", CanInteract: true},
	}
	repo.EXPECT().LoadModerationView(gomock.Any(), []moderationrepo.SubjectRef{ref}, viewer).Return(want, nil)

	got, err := newApplicationService(repo, &processorStub{}, &classifierStub{}, &deciderStub{}, zap.NewNop()).
		LoadViews(context.Background(), []moderation.SubjectRef{ref}, viewer)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}
