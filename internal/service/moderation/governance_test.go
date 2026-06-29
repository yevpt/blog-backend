package moderation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	repositorymock "github.com/vpt/blog-backend/internal/repository/moderation/mock"
	"github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/mock/gomock"
)

func TestEvaluateTrustPromotesOnlyCleanEligibleProfiles(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cfg := governanceConfig()

	tests := []struct {
		name    string
		profile moderation.UserModerationProfile
		want    moderation.TrustLevel
	}{
		{
			name: "new to normal",
			profile: moderation.UserModerationProfile{
				TrustLevel: moderation.TrustNew, CleanApprovalStreak: 3,
				CreatedAt: now.AddDate(0, 0, -7),
			},
			want: moderation.TrustNormal,
		},
		{
			name: "normal to trusted",
			profile: moderation.UserModerationProfile{
				TrustLevel: moderation.TrustNormal, CleanApprovalStreak: 20,
				CreatedAt: now.AddDate(0, 0, -30),
			},
			want: moderation.TrustTrusted,
		},
		{
			name: "violation blocks promotion",
			profile: moderation.UserModerationProfile{
				TrustLevel: moderation.TrustNormal, CleanApprovalStreak: 20,
				ViolationScore: 1, CreatedAt: now.AddDate(0, 0, -30),
			},
			want: moderation.TrustNormal,
		},
		{
			name: "manual lock is unchanged",
			profile: moderation.UserModerationProfile{
				TrustLevel: moderation.TrustNew, ManualTrustLocked: true,
				CleanApprovalStreak: 100, CreatedAt: now.AddDate(-1, 0, 0),
			},
			want: moderation.TrustNew,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := moderation.EvaluateTrust(tt.profile, cfg, now)
			assert.Equal(t, tt.want, got.TrustLevel)
		})
	}
}

func TestGovernanceServiceGetProfilePersistsAutomaticPromotion(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	repo.EXPECT().LoadModerationProfile(gomock.Any(), uint64(42), now).Return(moderationrepo.ModerationProfile{
		UserID: 42, TrustLevel: moderationrepo.TrustNew, TrustSource: moderationrepo.TrustSourceAuto,
		SanctionState: moderationrepo.SanctionActive, CleanApprovalStreak: 3,
		CreatedAt: now.AddDate(0, 0, -7), UpdatedAt: now,
	}, nil)
	repo.EXPECT().SetAutomaticTrust(gomock.Any(), moderationrepo.AutomaticTrustCommand{
		UserID: 42, TrustLevel: moderationrepo.TrustNormal, UpdatedAt: now,
	}).Return(true, nil)
	service := moderation.NewGovernanceService(repo, governanceConfig(), func() time.Time { return now })

	got, err := service.GetProfile(context.Background(), 42)

	require.NoError(t, err)
	assert.Equal(t, moderation.TrustNormal, got.TrustLevel)
}

func TestGovernanceServiceSetSanctionRejectsPastDeadline(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	service := moderation.NewGovernanceService(repo, governanceConfig(), func() time.Time { return now })
	past := now.Add(-time.Minute)

	err := service.SetSanction(context.Background(), moderation.SetSanctionCommand{
		UserID: 42, State: moderation.SanctionMuted, Until: &past, Reason: "测试",
	})

	require.ErrorIs(t, err, moderation.ErrInvalidRequest)
}

func TestGovernanceServiceSetAndReleaseSanctionRecordsActor(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	until := now.Add(24 * time.Hour)
	reason := "反复发送广告"
	repo.EXPECT().EnsureNewProfile(gomock.Any(), uint64(42), now).Return(nil)
	repo.EXPECT().SetSanction(gomock.Any(), moderationrepo.SetSanctionCommand{
		UserID: 42, ActorID: 1, State: moderationrepo.SanctionMuted,
		Until: &until, Reason: &reason, Now: now,
	}).Return(nil)
	repo.EXPECT().ReleaseSanction(gomock.Any(), moderationrepo.ReleaseSanctionCommand{
		UserID: 42, ActorID: 1, Now: now,
	}).Return(nil)
	service := moderation.NewGovernanceService(repo, governanceConfig(), func() time.Time { return now })

	require.NoError(t, service.SetSanction(context.Background(), moderation.SetSanctionCommand{
		UserID: 42, ActorID: 1, State: moderation.SanctionMuted, Until: &until, Reason: reason,
	}))
	require.NoError(t, service.ReleaseSanction(context.Background(), 42, 1))
}

func TestGovernanceServiceSetTrustCreatesMissingProfileBeforeManualLock(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	repo.EXPECT().EnsureNewProfile(gomock.Any(), uint64(42), now).Return(nil)
	repo.EXPECT().SetTrust(gomock.Any(), moderationrepo.SetTrustCommand{
		UserID: 42, ActorID: 1, TrustLevel: moderationrepo.TrustTrusted, ManualLocked: true, UpdatedAt: now,
	}).Return(nil)
	service := moderation.NewGovernanceService(repo, governanceConfig(), func() time.Time { return now })

	err := service.SetTrust(context.Background(), moderation.SetTrustCommand{
		UserID: 42, ActorID: 1, TrustLevel: moderation.TrustTrusted, ManualLocked: true,
	})

	require.NoError(t, err)
}

func TestGovernanceServiceSetTrustRejectsExpiredRestriction(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	service := moderation.NewGovernanceService(repo, governanceConfig(), func() time.Time { return now })

	err := service.SetTrust(context.Background(), moderation.SetTrustCommand{
		UserID: 42, ActorID: 1, TrustLevel: moderation.TrustRestricted,
		ManualLocked: true, RestrictedUntil: &past,
	})

	require.ErrorIs(t, err, moderation.ErrInvalidRequest)
}

func TestGovernanceServiceRegistrationAllowedUsesControlSingleton(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	repo.EXPECT().LoadControl(gomock.Any()).Return(moderationrepo.ControlRecord{
		RegistrationMode: moderationrepo.RegistrationClosed,
		PublishingMode:   moderationrepo.PublishingOpen,
	}, nil)
	service := moderation.NewGovernanceService(repo, governanceConfig(), time.Now)

	allowed, err := service.RegistrationAllowed(context.Background())

	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestEvaluateTrustRestrictsAndExtendsFromLatestViolation(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cfg := governanceConfig()
	violationAt := now.Add(-time.Hour)

	got := moderation.EvaluateTrust(moderation.UserModerationProfile{
		TrustLevel: moderation.TrustNormal, ViolationScore: 6,
		LastViolationAt: &violationAt, CreatedAt: now.AddDate(0, -2, 0),
	}, cfg, now)

	assert.Equal(t, moderation.TrustRestricted, got.TrustLevel)
	if assert.NotNil(t, got.RestrictedUntil) {
		assert.Equal(t, violationAt.Add(168*time.Hour), *got.RestrictedUntil)
	}
}

func governanceConfig() config.ModerationGovernanceConfig {
	return config.ModerationGovernanceConfig{
		NewToNormal:              config.ModerationPromotionConfig{MinAgeDays: 7, CleanApprovals: 3},
		NormalToTrusted:          config.ModerationPromotionConfig{MinAgeDays: 30, CleanApprovals: 20},
		RestrictedScoreThreshold: 6,
		RestrictedDuration:       168 * time.Hour,
	}
}
