package config

import (
	"fmt"
	"time"
)

const (
	// ModerationModeEnforce 表示审核决策实际生效。
	ModerationModeEnforce = "enforce"
	// ModerationModeObserve 表示仅观察规则命中，限本地调试使用。
	ModerationModeObserve = "observe"

	// ModerationActionAutoApprove 表示内容直接通过。
	ModerationActionAutoApprove = "auto_approve"
	// ModerationActionPostReview 表示内容先展示后审核。
	ModerationActionPostReview = "post_review"
	// ModerationActionPreReview 表示内容审核通过后展示。
	ModerationActionPreReview = "pre_review"
	// ModerationActionBlock 表示拒绝内容提交。
	ModerationActionBlock = "block"
)

// ModerationConfig 是内容审核的完整强类型配置。
type ModerationConfig struct {
	Enabled    bool                       `mapstructure:"enabled"`
	Mode       string                     `mapstructure:"mode"`
	Policy     ModerationPolicyConfig     `mapstructure:"policy"`
	Rules      ModerationRulesConfig      `mapstructure:"rules"`
	Content    ModerationContentConfig    `mapstructure:"content"`
	Image      ModerationImageConfig      `mapstructure:"image"`
	Governance ModerationGovernanceConfig `mapstructure:"governance"`
	RateLimit  ModerationRateLimitConfig  `mapstructure:"rate_limit"`
	Control    ModerationControlConfig    `mapstructure:"control"`
	Audit      ModerationAuditConfig      `mapstructure:"audit"`
	Notices    ModerationNoticesConfig    `mapstructure:"notices"`
}

// ModerationPolicyConfig 按用户信任等级定义审核动作。
type ModerationPolicyConfig struct {
	New        ModerationPolicyActionsConfig `mapstructure:"new"`
	Normal     ModerationPolicyActionsConfig `mapstructure:"normal"`
	Trusted    ModerationPolicyActionsConfig `mapstructure:"trusted"`
	Restricted ModerationPolicyActionsConfig `mapstructure:"restricted"`
}

// ModerationPolicyActionsConfig 定义一个信任等级在各风险场景下的动作。
type ModerationPolicyActionsConfig struct {
	CleanLow         string `mapstructure:"clean_low"`
	UnapprovedImage  string `mapstructure:"unapproved_image"`
	ExternalLinkOrAd string `mapstructure:"external_link_or_ad"`
	Medium           string `mapstructure:"medium"`
	High             string `mapstructure:"high"`
}

// ModerationRulesConfig 定义审核规则集的安全边界。
type ModerationRulesConfig struct {
	MaxPatternChars          int  `mapstructure:"max_pattern_chars"`
	MaxEnabledRegexRules     int  `mapstructure:"max_enabled_regex_rules"`
	RequireNonEmptyInEnforce bool `mapstructure:"require_non_empty_in_enforce"`
}

// ModerationContentConfig 定义各内容类型及附件数量上限。
type ModerationContentConfig struct {
	MomentMaxChars      int `mapstructure:"moment_max_chars"`
	CommentMaxChars     int `mapstructure:"comment_max_chars"`
	GuestbookMaxChars   int `mapstructure:"guestbook_max_chars"`
	ReplyMaxChars       int `mapstructure:"reply_max_chars"`
	MaxImagesPerContent int `mapstructure:"max_images_per_content"`
	MaxLinksPerContent  int `mapstructure:"max_links_per_content"`
}

// ModerationImageConfig 预留图片审核阶段的处理与清理边界。
type ModerationImageConfig struct {
	MaxUploadBytes        int64         `mapstructure:"max_upload_bytes"`
	MaxGIFBytes           int64         `mapstructure:"max_gif_bytes"`
	MaxStoredBytes        int64         `mapstructure:"max_stored_bytes"`
	MaxPixels             int64         `mapstructure:"max_pixels"`
	ProcessingConcurrency int           `mapstructure:"processing_concurrency"`
	PreviewMaxEdge        int           `mapstructure:"preview_max_edge"`
	StaticPlaceholderKey  string        `mapstructure:"static_placeholder_key"`
	GIFPlaceholderKey     string        `mapstructure:"gif_placeholder_key"`
	ApprovalRetentionDays int           `mapstructure:"approval_retention_days"`
	TempRetention         time.Duration `mapstructure:"temp_retention"`
	OrphanMinAge          time.Duration `mapstructure:"orphan_min_age"`
	CleanupInterval       time.Duration `mapstructure:"cleanup_interval"`
	CleanupBatchSize      int           `mapstructure:"cleanup_batch_size"`
}

// ModerationGovernanceConfig 预留用户等级晋升与限制参数。
type ModerationGovernanceConfig struct {
	NewToNormal              ModerationPromotionConfig        `mapstructure:"new_to_normal"`
	NormalToTrusted          ModerationPromotionConfig        `mapstructure:"normal_to_trusted"`
	RestrictedScoreThreshold int                              `mapstructure:"restricted_score_threshold"`
	RestrictedDuration       time.Duration                    `mapstructure:"restricted_duration"`
	ViolationWeights         ModerationViolationWeightsConfig `mapstructure:"violation_weights"`
}

// ModerationPromotionConfig 定义信任等级晋升门槛。
type ModerationPromotionConfig struct {
	MinAgeDays     int `mapstructure:"min_age_days"`
	CleanApprovals int `mapstructure:"clean_approvals"`
}

// ModerationViolationWeightsConfig 定义违规行为计分权重。
type ModerationViolationWeightsConfig struct {
	Corrected       int `mapstructure:"corrected"`
	Rejected        int `mapstructure:"rejected"`
	HighRiskBlocked int `mapstructure:"high_risk_blocked"`
}

// ModerationRateLimitConfig 定义单用户审核相关操作限频。
type ModerationRateLimitConfig struct {
	PublishPerMinute    int `mapstructure:"publish_per_minute"`
	EditPerMinute       int `mapstructure:"edit_per_minute"`
	TempUploadPerMinute int `mapstructure:"temp_upload_per_minute"`
}

// ModerationControlConfig 预留全站注册、发布控制参数。
type ModerationControlConfig struct {
	DefaultRegistrationMode    string        `mapstructure:"default_registration_mode"`
	DefaultPublishingMode      string        `mapstructure:"default_publishing_mode"`
	CacheTTL                   time.Duration `mapstructure:"cache_ttl"`
	UserHideBatchSize          int           `mapstructure:"user_hide_batch_size"`
	UserHideMaxItemsPerRequest int           `mapstructure:"user_hide_max_items_per_request"`
}

// ModerationAuditConfig 预留审核记录保留与清理参数。
type ModerationAuditConfig struct {
	AttemptRetentionDays          int           `mapstructure:"attempt_retention_days"`
	ActionLogRetentionDays        int           `mapstructure:"action_log_retention_days"`
	ObsoleteRevisionRetentionDays int           `mapstructure:"obsolete_revision_retention_days"`
	CleanupInterval               time.Duration `mapstructure:"cleanup_interval"`
	CleanupBatchSize              int           `mapstructure:"cleanup_batch_size"`
}

// ModerationNoticesConfig 定义对外稳定审核提示。
type ModerationNoticesConfig struct {
	Approved       string `mapstructure:"approved"`
	LowSubmitted   string `mapstructure:"low_submitted"`
	ReviewRequired string `mapstructure:"review_required"`
	HighRejected   string `mapstructure:"high_rejected"`
}

// Validate 校验审核模式、固定策略不变量及所有安全边界。
func (c ModerationConfig) Validate(environment string) error {
	// 生产环境不得关闭审核或切换到观察模式。
	if environment == "production" || environment == "prod" {
		if !c.Enabled {
			return fmt.Errorf("moderation.enabled: production requires true")
		}
		if c.Mode != ModerationModeEnforce {
			return fmt.Errorf("moderation.mode: production requires %s", ModerationModeEnforce)
		}
	}

	// 所有环境都只接受明确支持的运行模式。
	if c.Mode != ModerationModeEnforce && c.Mode != ModerationModeObserve {
		return fmt.Errorf("moderation.mode: unsupported value %q", c.Mode)
	}
	if err := validateModerationPolicy(c.Policy); err != nil {
		return err
	}
	if err := validateModerationBounds(c); err != nil {
		return err
	}
	return validateModerationStrings(c)
}
