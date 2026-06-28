package moderation

import "errors"

var (
	// ErrContentTooLong 表示清洗后的纯文本超过业务字符上限。
	ErrContentTooLong = errors.New("content exceeds character limit")
	// ErrEmptyRuleset 表示 enforce 分类器没有可安全启用的规则。
	ErrEmptyRuleset = errors.New("moderation ruleset cannot be empty")
	// ErrInvalidPolicyContext 表示策略输入或配置包含不支持的值。
	ErrInvalidPolicyContext = errors.New("invalid moderation policy context")
	// ErrPolicyBlocked 表示策略拒绝本次发布或编辑。
	ErrPolicyBlocked = errors.New("moderation policy blocked content")
	// ErrRevisionCollision 表示新版本 ID 已被当前审核项的版本指针占用。
	ErrRevisionCollision = errors.New("moderation revision id collision")
	// ErrInvalidTransition 表示事件不适用于当前审核状态。
	ErrInvalidTransition = errors.New("invalid moderation state transition")
	// ErrAlreadyDeleted 表示删除终态拒绝任何非删除事件。
	ErrAlreadyDeleted = errors.New("moderation item already deleted")
	// ErrInvalidRequest 表示审核写入参数缺失或不符合内容类型约束。
	ErrInvalidRequest = errors.New("invalid moderation request")
	// ErrImageReviewUnavailable 表示图片审核阶段尚未启用。
	ErrImageReviewUnavailable = errors.New("content image review is unavailable")
	// ErrContentRiskRejected 表示内容因较高风险被明确拒绝。
	ErrContentRiskRejected = errors.New("content rejected because of risk")
	// ErrPublishingForbidden 表示用户处罚或全站控制阻止本次发布。
	ErrPublishingForbidden = errors.New("content publishing is forbidden")
	// ErrInteractionNotAllowed 表示目标内容尚在审核或不可公开互动。
	ErrInteractionNotAllowed = errors.New("moderation subject cannot be interacted with")
)
