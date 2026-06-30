package moderationrule

import "errors"

// 稳定错误，供 handler 映射统一响应。
var (
	ErrRulesetConflict      = errors.New("规则集版本冲突")
	ErrRuleLimit            = errors.New("规则数量超限")
	ErrIndexMemoryLimit     = errors.New("索引内存超限")
	ErrImportInvalid        = errors.New("导入文件无效")
	ErrImportReportNotFound = errors.New("导入错误报告不存在")
	ErrRuleNotFound         = errors.New("规则不存在")
	ErrCandidateNotFound    = errors.New("候选规则集不存在")
	ErrCandidateNotReady    = errors.New("候选规则集尚未就绪")
	ErrEmptyRuleset         = errors.New("规则集不能为空")
	ErrInvalidRule          = errors.New("规则内容无效")
	ErrDuplicateRule        = errors.New("规则重复")
	ErrBatchLimit           = errors.New("批量操作数量超限")
)
