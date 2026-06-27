package moderation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/config"
)

func TestPolicyVocabularyMatchesPersistenceAndConfig(t *testing.T) {
	assert.Equal(t, model.ModerationTrustNew, string(moderation.TrustNew))
	assert.Equal(t, model.ModerationTrustNormal, string(moderation.TrustNormal))
	assert.Equal(t, model.ModerationTrustTrusted, string(moderation.TrustTrusted))
	assert.Equal(t, model.ModerationTrustRestricted, string(moderation.TrustRestricted))
	assert.Equal(t, model.ModerationSanctionActive, string(moderation.SanctionActive))
	assert.Equal(t, model.ModerationSanctionMuted, string(moderation.SanctionMuted))
	assert.Equal(t, model.ModerationSanctionBanned, string(moderation.SanctionBanned))
	assert.Equal(t, model.ModerationPublishingOpen, string(moderation.PublishingOpen))
	assert.Equal(t, model.ModerationPublishingPreReviewAll, string(moderation.PublishingPreReviewAll))
	assert.Equal(t, model.ModerationPublishingClosed, string(moderation.PublishingClosed))
	assert.Equal(t, config.ModerationActionAutoApprove, string(moderation.ActionAutoApprove))
	assert.Equal(t, config.ModerationActionPostReview, string(moderation.ActionPostReview))
	assert.Equal(t, config.ModerationActionPreReview, string(moderation.ActionPreReview))
	assert.Equal(t, config.ModerationActionBlock, string(moderation.ActionBlock))
}

func TestPolicyMatrix(t *testing.T) {
	tests := []struct {
		trust moderation.TrustLevel
		risk  moderation.RiskLevel
		want  moderation.PolicyAction
	}{
		{moderation.TrustNew, moderation.RiskLow, moderation.ActionPostReview},
		{moderation.TrustNew, moderation.RiskMedium, moderation.ActionPreReview},
		{moderation.TrustNew, moderation.RiskHigh, moderation.ActionBlock},
		{moderation.TrustNormal, moderation.RiskLow, moderation.ActionPostReview},
		{moderation.TrustNormal, moderation.RiskMedium, moderation.ActionPreReview},
		{moderation.TrustNormal, moderation.RiskHigh, moderation.ActionBlock},
		{moderation.TrustTrusted, moderation.RiskLow, moderation.ActionAutoApprove},
		{moderation.TrustTrusted, moderation.RiskMedium, moderation.ActionPreReview},
		{moderation.TrustTrusted, moderation.RiskHigh, moderation.ActionBlock},
		{moderation.TrustRestricted, moderation.RiskLow, moderation.ActionPreReview},
		{moderation.TrustRestricted, moderation.RiskMedium, moderation.ActionPreReview},
		{moderation.TrustRestricted, moderation.RiskHigh, moderation.ActionBlock},
	}

	for _, tt := range tests {
		t.Run(string(tt.trust)+"/"+string(tt.risk), func(t *testing.T) {
			got, err := moderation.Decide(moderation.PolicyInput{
				Trust:      tt.trust,
				Sanction:   moderation.SanctionActive,
				Publishing: moderation.PublishingOpen,
				Risk:       tt.risk,
				Policy:     policyConfig(),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPolicyLowRiskSignalsUseMostRestrictiveAction(t *testing.T) {
	tests := []struct {
		name                string
		trust               moderation.TrustLevel
		hasUnapprovedImage  bool
		hasExternalLinkOrAd bool
		want                moderation.PolicyAction
	}{
		{name: "new image", trust: moderation.TrustNew, hasUnapprovedImage: true, want: moderation.ActionPreReview},
		{name: "normal image", trust: moderation.TrustNormal, hasUnapprovedImage: true, want: moderation.ActionPostReview},
		{name: "trusted image", trust: moderation.TrustTrusted, hasUnapprovedImage: true, want: moderation.ActionPostReview},
		{name: "trusted external link", trust: moderation.TrustTrusted, hasExternalLinkOrAd: true, want: moderation.ActionPreReview},
		{name: "trusted image and external link", trust: moderation.TrustTrusted, hasUnapprovedImage: true, hasExternalLinkOrAd: true, want: moderation.ActionPreReview},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := moderation.Decide(moderation.PolicyInput{
				Trust:               tt.trust,
				Sanction:            moderation.SanctionActive,
				Publishing:          moderation.PublishingOpen,
				Risk:                moderation.RiskLow,
				HasUnapprovedImage:  tt.hasUnapprovedImage,
				HasExternalLinkOrAd: tt.hasExternalLinkOrAd,
				Policy:              policyConfig(),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPolicyPriority(t *testing.T) {
	t.Run("admin bypass", func(t *testing.T) {
		got, err := moderation.Decide(moderation.PolicyInput{
			IsAdmin:    true,
			Trust:      moderation.TrustRestricted,
			Sanction:   moderation.SanctionBanned,
			Publishing: moderation.PublishingClosed,
			Risk:       moderation.RiskHigh,
			Policy:     policyConfig(),
		})
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionAutoApprove, got)
	})

	for _, sanction := range []moderation.SanctionState{moderation.SanctionMuted, moderation.SanctionBanned} {
		t.Run(string(sanction), func(t *testing.T) {
			got, err := moderation.Decide(basePolicyInput(moderation.TrustTrusted, moderation.RiskLow, moderation.PublishingOpen, sanction))
			require.NoError(t, err)
			assert.Equal(t, moderation.ActionBlock, got)
		})
	}

	t.Run("publishing closed", func(t *testing.T) {
		got, err := moderation.Decide(basePolicyInput(moderation.TrustTrusted, moderation.RiskLow, moderation.PublishingClosed, moderation.SanctionActive))
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionBlock, got)
	})

	t.Run("pre review all downgrades publish actions", func(t *testing.T) {
		for _, trust := range []moderation.TrustLevel{moderation.TrustNew, moderation.TrustNormal, moderation.TrustTrusted} {
			got, err := moderation.Decide(basePolicyInput(trust, moderation.RiskLow, moderation.PublishingPreReviewAll, moderation.SanctionActive))
			require.NoError(t, err)
			assert.Equal(t, moderation.ActionPreReview, got)
		}
	})
}

func TestPolicyHighRiskAlwaysBlocksOrdinaryUsers(t *testing.T) {
	policy := policyConfig()
	policy.Trusted.High = config.ModerationActionAutoApprove

	for _, trust := range []moderation.TrustLevel{moderation.TrustNew, moderation.TrustNormal, moderation.TrustTrusted, moderation.TrustRestricted} {
		input := basePolicyInput(trust, moderation.RiskHigh, moderation.PublishingOpen, moderation.SanctionActive)
		input.Policy = policy
		got, err := moderation.Decide(input)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionBlock, got)
	}
}

func TestPolicyRestrictedNeverPublishesWithoutReview(t *testing.T) {
	policy := policyConfig()
	policy.Restricted.CleanLow = config.ModerationActionAutoApprove
	policy.Restricted.Medium = config.ModerationActionPostReview

	for _, risk := range []moderation.RiskLevel{moderation.RiskLow, moderation.RiskMedium} {
		input := basePolicyInput(moderation.TrustRestricted, risk, moderation.PublishingOpen, moderation.SanctionActive)
		input.Policy = policy
		got, err := moderation.Decide(input)
		require.NoError(t, err)
		assert.Equal(t, moderation.ActionPreReview, got)
	}
}

func TestPolicyRejectsUnknownContext(t *testing.T) {
	tests := []struct {
		name  string
		input moderation.PolicyInput
	}{
		{name: "trust", input: basePolicyInput("unknown", moderation.RiskLow, moderation.PublishingOpen, moderation.SanctionActive)},
		{name: "risk", input: basePolicyInput(moderation.TrustNew, "unknown", moderation.PublishingOpen, moderation.SanctionActive)},
		{name: "sanction", input: basePolicyInput(moderation.TrustNew, moderation.RiskLow, moderation.PublishingOpen, "unknown")},
		{name: "publishing", input: basePolicyInput(moderation.TrustNew, moderation.RiskLow, "unknown", moderation.SanctionActive)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := moderation.Decide(tt.input)
			assert.ErrorIs(t, err, moderation.ErrInvalidPolicyContext)
		})
	}
}

func basePolicyInput(trust moderation.TrustLevel, risk moderation.RiskLevel, publishing moderation.PublishingMode, sanction moderation.SanctionState) moderation.PolicyInput {
	return moderation.PolicyInput{
		Trust:      trust,
		Sanction:   sanction,
		Publishing: publishing,
		Risk:       risk,
		Policy:     policyConfig(),
	}
}

func policyConfig() config.ModerationPolicyConfig {
	return config.ModerationPolicyConfig{
		New: config.ModerationPolicyActionsConfig{
			CleanLow: config.ModerationActionPostReview, UnapprovedImage: config.ModerationActionPreReview,
			ExternalLinkOrAd: config.ModerationActionPreReview, Medium: config.ModerationActionPreReview, High: config.ModerationActionBlock,
		},
		Normal: config.ModerationPolicyActionsConfig{
			CleanLow: config.ModerationActionPostReview, UnapprovedImage: config.ModerationActionPostReview,
			ExternalLinkOrAd: config.ModerationActionPostReview, Medium: config.ModerationActionPreReview, High: config.ModerationActionBlock,
		},
		Trusted: config.ModerationPolicyActionsConfig{
			CleanLow: config.ModerationActionAutoApprove, UnapprovedImage: config.ModerationActionPostReview,
			ExternalLinkOrAd: config.ModerationActionPreReview, Medium: config.ModerationActionPreReview, High: config.ModerationActionBlock,
		},
		Restricted: config.ModerationPolicyActionsConfig{
			CleanLow: config.ModerationActionPreReview, UnapprovedImage: config.ModerationActionPreReview,
			ExternalLinkOrAd: config.ModerationActionPreReview, Medium: config.ModerationActionPreReview, High: config.ModerationActionBlock,
		},
	}
}
