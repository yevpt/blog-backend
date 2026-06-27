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
	// ErrInvalidTransition 表示事件不适用于当前审核状态。
	ErrInvalidTransition = errors.New("invalid moderation state transition")
	// ErrAlreadyDeleted 表示删除终态拒绝任何非删除事件。
	ErrAlreadyDeleted = errors.New("moderation item already deleted")
)
