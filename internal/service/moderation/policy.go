package moderation

import (
	"fmt"

	"github.com/vpt/blog-backend/pkg/config"
)

// TrustLevel 表示参与审核决策的用户信任等级。
type TrustLevel string

const (
	TrustNew        TrustLevel = "new"
	TrustNormal     TrustLevel = "normal"
	TrustTrusted    TrustLevel = "trusted"
	TrustRestricted TrustLevel = "restricted"
)

// SanctionState 表示用户当前发布权限状态。
type SanctionState string

const (
	SanctionActive SanctionState = "active"
	SanctionMuted  SanctionState = "muted"
	SanctionBanned SanctionState = "banned"
)

// PublishingMode 表示全站当前发布控制模式。
type PublishingMode string

const (
	PublishingOpen         PublishingMode = "open"
	PublishingPreReviewAll PublishingMode = "pre_review_all"
	PublishingClosed       PublishingMode = "closed"
)

// PolicyAction 表示发布或编辑经过策略计算后的处理方式。
type PolicyAction string

const (
	ActionAutoApprove PolicyAction = config.ModerationActionAutoApprove
	ActionPostReview  PolicyAction = config.ModerationActionPostReview
	ActionPreReview   PolicyAction = config.ModerationActionPreReview
	ActionBlock       PolicyAction = config.ModerationActionBlock
)

// PolicyInput 汇总一次策略决策所需的用户、全站控制和内容信号。
type PolicyInput struct {
	IsAdmin             bool
	Trust               TrustLevel
	Sanction            SanctionState
	Publishing          PublishingMode
	Risk                RiskLevel
	HasUnapprovedImage  bool
	HasExternalLinkOrAd bool
	Policy              config.ModerationPolicyConfig
}

// Decide 按固定优先级返回策略动作，管理员是唯一绕过普通用户限制的角色。
func Decide(input PolicyInput) (PolicyAction, error) {
	// 管理员发布不进入普通用户风险和全站控制分支。
	if input.IsAdmin {
		return ActionAutoApprove, nil
	}

	// 处罚优先于全站模式和信任等级。
	if !validSanction(input.Sanction) {
		return "", invalidPolicyValue("sanction", input.Sanction)
	}
	if input.Sanction == SanctionMuted || input.Sanction == SanctionBanned {
		return ActionBlock, nil
	}

	// 关闭发布直接阻断普通用户。
	if !validPublishingMode(input.Publishing) {
		return "", invalidPolicyValue("publishing", input.Publishing)
	}
	if input.Publishing == PublishingClosed {
		return ActionBlock, nil
	}

	// 高风险是不受配置降级影响的硬阻断边界。
	if !validRisk(input.Risk) {
		return "", invalidPolicyValue("risk", input.Risk)
	}
	if input.Risk == RiskHigh {
		return ActionBlock, nil
	}

	// 从用户等级对应的强类型配置选择基础动作。
	level, err := policyForTrust(input.Policy, input.Trust)
	if err != nil {
		return "", err
	}
	action, err := actionForSignals(level, input)
	if err != nil {
		return "", err
	}

	// 受限用户和全站强制预审都不能直接或先发后审。
	if input.Trust == TrustRestricted || input.Publishing == PublishingPreReviewAll {
		return requirePreReview(action), nil
	}
	return action, nil
}

func policyForTrust(policy config.ModerationPolicyConfig, trust TrustLevel) (config.ModerationPolicyActionsConfig, error) {
	switch trust {
	case TrustNew:
		return policy.New, nil
	case TrustNormal:
		return policy.Normal, nil
	case TrustTrusted:
		return policy.Trusted, nil
	case TrustRestricted:
		return policy.Restricted, nil
	default:
		return config.ModerationPolicyActionsConfig{}, invalidPolicyValue("trust", trust)
	}
}

func actionForSignals(level config.ModerationPolicyActionsConfig, input PolicyInput) (PolicyAction, error) {
	if input.Risk == RiskMedium {
		return parsePolicyAction(level.Medium)
	}

	// 低风险同时命中图片和链接信号时选择更严格的配置动作。
	values := []string{level.CleanLow}
	if input.HasUnapprovedImage {
		values = append(values, level.UnapprovedImage)
	}
	if input.HasExternalLinkOrAd {
		values = append(values, level.ExternalLinkOrAd)
	}
	return mostRestrictiveAction(values)
}

func mostRestrictiveAction(values []string) (PolicyAction, error) {
	action := ActionAutoApprove
	for _, value := range values {
		candidate, err := parsePolicyAction(value)
		if err != nil {
			return "", err
		}
		if actionRank(candidate) > actionRank(action) {
			action = candidate
		}
	}
	return action, nil
}

func parsePolicyAction(value string) (PolicyAction, error) {
	action := PolicyAction(value)
	switch action {
	case ActionAutoApprove, ActionPostReview, ActionPreReview, ActionBlock:
		return action, nil
	default:
		return "", invalidPolicyValue("action", value)
	}
}

func actionRank(action PolicyAction) int {
	switch action {
	case ActionAutoApprove:
		return 0
	case ActionPostReview:
		return 1
	case ActionPreReview:
		return 2
	case ActionBlock:
		return 3
	default:
		return -1
	}
}

func requirePreReview(action PolicyAction) PolicyAction {
	if action == ActionAutoApprove || action == ActionPostReview {
		return ActionPreReview
	}
	return action
}

func validSanction(state SanctionState) bool {
	return state == SanctionActive || state == SanctionMuted || state == SanctionBanned
}

func validPublishingMode(mode PublishingMode) bool {
	return mode == PublishingOpen || mode == PublishingPreReviewAll || mode == PublishingClosed
}

func validRisk(risk RiskLevel) bool {
	return risk == RiskLow || risk == RiskMedium || risk == RiskHigh
}

func invalidPolicyValue(field string, value any) error {
	return fmt.Errorf("%w: unsupported %s %q", ErrInvalidPolicyContext, field, value)
}
