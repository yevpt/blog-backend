package config

import (
	"fmt"
	"strings"
)

func validateModerationPolicy(policy ModerationPolicyConfig) error {
	levels := []struct {
		name       string
		actions    ModerationPolicyActionsConfig
		restricted bool
	}{
		{name: "new", actions: policy.New},
		{name: "normal", actions: policy.Normal},
		{name: "trusted", actions: policy.Trusted},
		{name: "restricted", actions: policy.Restricted, restricted: true},
	}

	for _, level := range levels {
		if err := validateModerationPolicyLevel(level.name, level.actions, level.restricted); err != nil {
			return err
		}
	}
	return nil
}

func validateModerationPolicyLevel(name string, actions ModerationPolicyActionsConfig, restricted bool) error {
	fields := []struct {
		name   string
		action string
	}{
		{name: "clean_low", action: actions.CleanLow},
		{name: "unapproved_image", action: actions.UnapprovedImage},
		{name: "external_link_or_ad", action: actions.ExternalLinkOrAd},
		{name: "medium", action: actions.Medium},
		{name: "high", action: actions.High},
	}

	for _, field := range fields {
		path := fmt.Sprintf("moderation.policy.%s.%s", name, field.name)
		if !isModerationAction(field.action) {
			return fmt.Errorf("%s: unsupported action %q", path, field.action)
		}
		if restricted && (field.action == ModerationActionAutoApprove || field.action == ModerationActionPostReview) {
			return fmt.Errorf("%s: restricted users require pre_review or block", path)
		}
	}
	if actions.High != ModerationActionBlock {
		return fmt.Errorf("moderation.policy.%s.high: high risk must be blocked", name)
	}
	if actions.UnapprovedImage == ModerationActionAutoApprove {
		return fmt.Errorf("moderation.policy.%s.unapproved_image: unapproved images cannot be auto-approved", name)
	}
	return nil
}

func isModerationAction(action string) bool {
	switch action {
	case ModerationActionAutoApprove, ModerationActionPostReview, ModerationActionPreReview, ModerationActionBlock:
		return true
	default:
		return false
	}
}

func validateModerationBounds(c ModerationConfig) error {
	values := []struct {
		path  string
		value int64
	}{
		{path: "moderation.rules.max_pattern_chars", value: int64(c.Rules.MaxPatternChars)},
		{path: "moderation.rules.max_enabled_regex_rules", value: int64(c.Rules.MaxEnabledRegexRules)},
		{path: "moderation.content.moment_max_chars", value: int64(c.Content.MomentMaxChars)},
		{path: "moderation.content.comment_max_chars", value: int64(c.Content.CommentMaxChars)},
		{path: "moderation.content.guestbook_max_chars", value: int64(c.Content.GuestbookMaxChars)},
		{path: "moderation.content.reply_max_chars", value: int64(c.Content.ReplyMaxChars)},
		{path: "moderation.content.max_images_per_content", value: int64(c.Content.MaxImagesPerContent)},
		{path: "moderation.content.max_links_per_content", value: int64(c.Content.MaxLinksPerContent)},
		{path: "moderation.review.queue_default_page_size", value: int64(c.Review.QueueDefaultPageSize)},
		{path: "moderation.review.queue_max_page_size", value: int64(c.Review.QueueMaxPageSize)},
		{path: "moderation.review.reason_max_chars", value: int64(c.Review.ReasonMaxChars)},
		{path: "moderation.image.max_upload_bytes", value: c.Image.MaxUploadBytes},
		{path: "moderation.image.max_gif_bytes", value: c.Image.MaxGIFBytes},
		{path: "moderation.image.max_stored_bytes", value: c.Image.MaxStoredBytes},
		{path: "moderation.image.max_pixels", value: c.Image.MaxPixels},
		{path: "moderation.image.processing_concurrency", value: int64(c.Image.ProcessingConcurrency)},
		{path: "moderation.image.preview_max_edge", value: int64(c.Image.PreviewMaxEdge)},
		{path: "moderation.image.approval_retention_days", value: int64(c.Image.ApprovalRetentionDays)},
		{path: "moderation.image.temp_retention", value: int64(c.Image.TempRetention)},
		{path: "moderation.image.orphan_min_age", value: int64(c.Image.OrphanMinAge)},
		{path: "moderation.image.cleanup_interval", value: int64(c.Image.CleanupInterval)},
		{path: "moderation.image.cleanup_batch_size", value: int64(c.Image.CleanupBatchSize)},
		{path: "moderation.governance.new_to_normal.min_age_days", value: int64(c.Governance.NewToNormal.MinAgeDays)},
		{path: "moderation.governance.new_to_normal.clean_approvals", value: int64(c.Governance.NewToNormal.CleanApprovals)},
		{path: "moderation.governance.normal_to_trusted.min_age_days", value: int64(c.Governance.NormalToTrusted.MinAgeDays)},
		{path: "moderation.governance.normal_to_trusted.clean_approvals", value: int64(c.Governance.NormalToTrusted.CleanApprovals)},
		{path: "moderation.governance.restricted_score_threshold", value: int64(c.Governance.RestrictedScoreThreshold)},
		{path: "moderation.governance.restricted_duration", value: int64(c.Governance.RestrictedDuration)},
		{path: "moderation.governance.violation_weights.corrected", value: int64(c.Governance.ViolationWeights.Corrected)},
		{path: "moderation.governance.violation_weights.rejected", value: int64(c.Governance.ViolationWeights.Rejected)},
		{path: "moderation.governance.violation_weights.high_risk_blocked", value: int64(c.Governance.ViolationWeights.HighRiskBlocked)},
		{path: "moderation.rate_limit.publish_per_minute", value: int64(c.RateLimit.PublishPerMinute)},
		{path: "moderation.rate_limit.edit_per_minute", value: int64(c.RateLimit.EditPerMinute)},
		{path: "moderation.rate_limit.temp_upload_per_minute", value: int64(c.RateLimit.TempUploadPerMinute)},
		{path: "moderation.control.cache_ttl", value: int64(c.Control.CacheTTL)},
		{path: "moderation.control.user_hide_batch_size", value: int64(c.Control.UserHideBatchSize)},
		{path: "moderation.control.user_hide_max_items_per_request", value: int64(c.Control.UserHideMaxItemsPerRequest)},
		{path: "moderation.audit.attempt_retention_days", value: int64(c.Audit.AttemptRetentionDays)},
		{path: "moderation.audit.action_log_retention_days", value: int64(c.Audit.ActionLogRetentionDays)},
		{path: "moderation.audit.obsolete_revision_retention_days", value: int64(c.Audit.ObsoleteRevisionRetentionDays)},
		{path: "moderation.audit.cleanup_interval", value: int64(c.Audit.CleanupInterval)},
		{path: "moderation.audit.cleanup_batch_size", value: int64(c.Audit.CleanupBatchSize)},
	}

	for _, item := range values {
		if item.value <= 0 {
			return fmt.Errorf("%s: must be positive", item.path)
		}
	}
	if c.Review.QueueDefaultPageSize > c.Review.QueueMaxPageSize {
		return fmt.Errorf("moderation.review.queue_default_page_size: must not exceed queue_max_page_size")
	}
	if c.Review.QueueMaxPageSize > 500 {
		return fmt.Errorf("moderation.review.queue_max_page_size: must not exceed 500")
	}
	if c.Review.ReasonMaxChars > 1000 {
		return fmt.Errorf("moderation.review.reason_max_chars: must not exceed database capacity 1000")
	}
	return nil
}

func validateModerationStrings(c ModerationConfig) error {
	values := []struct {
		path  string
		value string
	}{
		{path: "moderation.image.static_placeholder_key", value: c.Image.StaticPlaceholderKey},
		{path: "moderation.image.gif_placeholder_key", value: c.Image.GIFPlaceholderKey},
		{path: "moderation.notices.approved", value: c.Notices.Approved},
		{path: "moderation.notices.low_submitted", value: c.Notices.LowSubmitted},
		{path: "moderation.notices.review_required", value: c.Notices.ReviewRequired},
		{path: "moderation.notices.high_rejected", value: c.Notices.HighRejected},
	}

	for _, item := range values {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%s: must not be empty", item.path)
		}
	}
	if !isOneOf(c.Control.DefaultRegistrationMode, "open", "closed") {
		return fmt.Errorf("moderation.control.default_registration_mode: unsupported value %q", c.Control.DefaultRegistrationMode)
	}
	if !isOneOf(c.Control.DefaultPublishingMode, "open", "pre_review_all", "closed") {
		return fmt.Errorf("moderation.control.default_publishing_mode: unsupported value %q", c.Control.DefaultPublishingMode)
	}
	return nil
}

func isOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
