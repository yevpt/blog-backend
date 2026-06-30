package dto

import "time"

// AdminModerationRuleListReq 是管理端规则列表查询参数。
type AdminModerationRuleListReq struct {
	Cursor     uint64 `form:"cursor" example:"100"`
	Limit      int    `form:"limit" binding:"omitempty,min=1,max=100" example:"50"`
	ID         uint64 `form:"id" example:"42"`
	Pattern    string `form:"pattern" binding:"omitempty,max=500" example:"风险"`
	SearchMode string `form:"search_mode" binding:"omitempty,oneof=exact prefix" example:"prefix"`
	Category   string `form:"category" example:"fraud"`
	RuleType   string `form:"rule_type" example:"keyword"`
	RiskLevel  string `form:"risk_level" example:"medium"`
	Effect     string `form:"effect" example:"review"`
	SourceID   uint64 `form:"source_id" example:"1"`
	Active     *bool  `form:"active" example:"true"`
}

// AdminModerationRuleSaveReq 是新增或替代规则的请求体。
type AdminModerationRuleSaveReq struct {
	ExpectedRulesetVersion uint64  `json:"expected_ruleset_version" binding:"required" example:"7"`
	Name                   *string `json:"name" binding:"omitempty,max=100" example:"涉政词"`
	RuleType               string  `json:"rule_type" binding:"required,oneof=keyword regexp composite" example:"keyword"`
	Pattern                string  `json:"pattern" binding:"required,max=500" example:"风险词"`
	Category               string  `json:"category" binding:"required" example:"other"`
	Effect                 string  `json:"effect" binding:"required,oneof=review allow" example:"review"`
	RiskLevel              string  `json:"risk_level" binding:"required,oneof=low medium high" example:"medium"`
	Priority               int32   `json:"priority" example:"100"`
	SourceID               uint64  `json:"source_id" binding:"required" example:"1"`
}

// AdminModerationRuleBatchStatusReq 是批量启停规则请求体。
type AdminModerationRuleBatchStatusReq struct {
	ExpectedRulesetVersion uint64   `json:"expected_ruleset_version" binding:"required" example:"7"`
	RuleIDs                []uint64 `json:"rule_ids" binding:"required,min=1,max=1000,dive,required" example:"1,2,3"`
	Active                 bool     `json:"active" example:"false"`
}

// AdminModerationRuleTestReq 是文本试跑请求体。
type AdminModerationRuleTestReq struct {
	Text      string  `json:"text" binding:"required,max=10000" example:"这是一段测试文本"`
	RulesetID *uint64 `json:"ruleset_id" example:"8"`
}

// AdminModerationRuleImportPublishReq 是导入发布确认请求体。
type AdminModerationRuleImportPublishReq struct {
	ExpectedRulesetVersion uint64 `json:"expected_ruleset_version" binding:"required" example:"7"`
}

// AdminModerationRuleImportCreateReq 是导入任务创建的表单参数。
type AdminModerationRuleImportCreateReq struct {
	Format           string `form:"format" binding:"required,oneof=csv txt" example:"csv"`
	SourceName       string `form:"source_name" binding:"required,min=1,max=100" example:"采购词库"`
	DefaultCategory  string `form:"default_category" binding:"required" example:"fraud"`
	DefaultEffect    string `form:"default_effect" binding:"required,oneof=review allow" example:"review"`
	DefaultRiskLevel string `form:"default_risk_level" binding:"required,oneof=low medium high" example:"medium"`
	DefaultPriority  int32  `form:"default_priority" example:"100"`
}

// AdminModerationRuleResp 是单条规则响应。
type AdminModerationRuleResp struct {
	ID                   uint64  `json:"id" example:"42"`
	Name                 *string `json:"name,omitempty"`
	RuleType             string  `json:"rule_type" example:"keyword"`
	Pattern              string  `json:"pattern" example:"风险词"`
	Category             string  `json:"category" example:"fraud"`
	Effect               string  `json:"effect" example:"review"`
	RiskLevel            string  `json:"risk_level" example:"medium"`
	Priority             int32   `json:"priority" example:"100"`
	SourceID             uint64  `json:"source_id" example:"1"`
	ActivatedRulesetID   uint64  `json:"activated_ruleset_id" example:"7"`
	DeactivatedRulesetID *uint64 `json:"deactivated_ruleset_id,omitempty"`
	ReplacesRuleID       *uint64 `json:"replaces_rule_id,omitempty"`
	Active               bool    `json:"active" example:"true"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// AdminModerationRulePageResp 是规则游标分页响应。
type AdminModerationRulePageResp struct {
	List       []AdminModerationRuleResp `json:"list"`
	NextCursor uint64                    `json:"next_cursor"`
	HasMore    bool                      `json:"has_more"`
}

// AdminModerationRuleMetadataResp 是规则目录元数据响应。
type AdminModerationRuleMetadataResp struct {
	Categories []AdminModerationCategoryEntry `json:"categories"`
	RiskLevels []string                       `json:"risk_levels"`
	Effects    []string                       `json:"effects"`
	RuleTypes  []string                       `json:"rule_types"`
	Sources    []AdminModerationSourceEntry   `json:"sources"`
}

// AdminModerationCategoryEntry 是受控分类的稳定键和显示名。
type AdminModerationCategoryEntry struct {
	Key  string `json:"key" example:"fraud"`
	Name string `json:"name" example:"欺诈"`
}

// AdminModerationSourceEntry 是规则来源目录条目。
type AdminModerationSourceEntry struct {
	ID   uint64 `json:"id" example:"1"`
	Name string `json:"name" example:"手工维护"`
}

// AdminModerationRuleStatusResp 是规则集状态响应。
type AdminModerationRuleStatusResp struct {
	CurrentRulesetID uint64                                `json:"current_ruleset_id" example:"7"`
	RuleCount        uint64                                `json:"rule_count" example:"1000"`
	KeywordCount     uint64                                `json:"keyword_count" example:"950"`
	RegexpCount      uint64                                `json:"regexp_count" example:"30"`
	CompositeCount   uint64                                `json:"composite_count" example:"20"`
	IndexBytes       uint64                                `json:"index_bytes" example:"4096"`
	BuildPeakBytes   uint64                                `json:"build_peak_bytes" example:"8192"`
	BuildDurationMS  uint64                                `json:"build_duration_ms" example:"500"`
	UpdatedAt        time.Time                             `json:"updated_at"`
	Candidate        *AdminModerationCandidateStatusResp   `json:"candidate,omitempty"`
}

// AdminModerationCandidateStatusResp 是候选规则集状态。
type AdminModerationCandidateStatusResp struct {
	RulesetID      uint64  `json:"ruleset_id" example:"8"`
	Status         string  `json:"status" example:"ready"`
	BaseRulesetID  uint64  `json:"base_ruleset_id" example:"7"`
	RuleCount      uint64  `json:"rule_count" example:"1100"`
	IndexBytes     uint64  `json:"index_bytes" example:"4500"`
	BuildPeakBytes uint64  `json:"build_peak_bytes" example:"9000"`
	FailureCode    *string `json:"failure_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AdminModerationRuleJobResp 是异步任务响应。
type AdminModerationRuleJobResp struct {
	RulesetID     uint64 `json:"ruleset_id" example:"8"`
	BaseRulesetID uint64 `json:"base_ruleset_id" example:"7"`
	Status        string `json:"status" example:"building"`
}

// AdminModerationRuleTestHitResp 是试跑单条命中详情。
type AdminModerationRuleTestHitResp struct {
	RuleID    uint64 `json:"rule_id" example:"5"`
	RuleType  string `json:"rule_type" example:"keyword"`
	Pattern   string `json:"pattern" example:"风险"`
	Category  string `json:"category" example:"fraud"`
	RiskLevel string `json:"risk_level" example:"high"`
	Effect    string `json:"effect" example:"review"`
	Excerpt   string `json:"excerpt" example:"包含风险内容"`
}

// AdminModerationRuleTestResp 是文本试跑结果响应。
type AdminModerationRuleTestResp struct {
	Risk          string                              `json:"risk" example:"high"`
	RulesetID     uint64                              `json:"ruleset_id" example:"7"`
	RuleIDs       []uint64                            `json:"rule_ids"`
	SuppressedIDs []uint64                            `json:"suppressed_ids"`
	Truncated     bool                                `json:"truncated"`
	Hits          []AdminModerationRuleTestHitResp    `json:"hits"`
}

// AdminModerationImportResp 是导入任务响应。
type AdminModerationImportResp struct {
	ID               uint64  `json:"id" example:"1"`
	FileName         string  `json:"file_name" example:"rules.csv"`
	Format           string  `json:"format" example:"csv"`
	FileSize         uint64  `json:"file_size" example:"1024"`
	SourceID         uint64  `json:"source_id" example:"1"`
	DefaultCategory  string  `json:"default_category" example:"fraud"`
	DefaultEffect    string  `json:"default_effect" example:"review"`
	DefaultRiskLevel string  `json:"default_risk_level" example:"medium"`
	DefaultPriority  int32   `json:"default_priority" example:"100"`
	ValidationStatus string  `json:"validation_status" example:"valid"`
	TotalRows        uint64  `json:"total_rows" example:"1000"`
	ValidRows        uint64  `json:"valid_rows" example:"950"`
	DuplicateRows    uint64  `json:"duplicate_rows" example:"10"`
	ErrorRows        uint64  `json:"error_rows" example:"40"`
	ErrorObjectKey   *string `json:"error_object_key,omitempty"`
	RulesetID        *uint64 `json:"ruleset_id,omitempty"`
	OperatorID       uint64  `json:"operator_id" example:"1"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AdminModerationImportPageResp 是导入历史分页响应。
type AdminModerationImportPageResp struct {
	List       []AdminModerationImportResp `json:"list"`
	NextCursor uint64                      `json:"next_cursor"`
	HasMore    bool                        `json:"has_more"`
}
