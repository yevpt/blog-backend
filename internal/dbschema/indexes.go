package dbschema

import (
	"fmt"

	"gorm.io/gorm"
)

const (
	userLikeLookupIndexName       = "idx_user_like_type_target_active"
	userLikeLookupIndexColumnsSQL = "`type`, `target_id`, `deleted_at`"
	userLikeLookupIndexSQL        = "CREATE INDEX `" + userLikeLookupIndexName + "` ON `user_like` (" + userLikeLookupIndexColumnsSQL + ")"
	schemaIndexExistsQuery        = "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?"
)

// ApplySchemaIndexes 补充模型标签无法表达的复合索引。
func ApplySchemaIndexes(db *gorm.DB) error {
	exists, err := schemaIndexExists(db, "user_like", userLikeLookupIndexName)
	if err != nil {
		return fmt.Errorf("检查索引 %s: %w", userLikeLookupIndexName, err)
	}
	if exists {
		return nil
	}
	if err := db.Exec(userLikeLookupIndexSQL).Error; err != nil {
		return fmt.Errorf("创建索引 %s: %w", userLikeLookupIndexName, err)
	}
	return nil
}

func schemaIndexExists(db *gorm.DB, table, index string) (bool, error) {
	var count int64
	if err := db.Raw(schemaIndexExistsQuery, table, index).Row().Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
