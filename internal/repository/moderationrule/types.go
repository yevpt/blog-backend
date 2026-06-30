package moderationrule

// RulesetRecord 是当前已发布规则集的启动元数据。
type RulesetRecord struct {
	ID                 uint64
	Status             string
	IndexObjectKey     *string
	IndexFormatVersion uint32
	IndexSHA256        *string
	IndexBytes         uint64
}

// RuleRecord 是构建运行时索引所需的最小规则事实。
type RuleRecord struct {
	ID        uint64
	RuleType  string
	Pattern   string
	RiskLevel string
	Effect    string
	Priority  int32
}
