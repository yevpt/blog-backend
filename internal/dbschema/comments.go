package dbschema

import "strings"

// SchemaCommentSet 保存数据库表与字段注释。
type SchemaCommentSet struct {
	Tables map[string]TableComment
}

// TableComment 保存单张表的注释信息。
type TableComment struct {
	Comment string
	Columns map[string]string
}

var schemaComments = map[string]TableComment{}

// SchemaComments 返回当前数据库注释 catalog。
func SchemaComments() SchemaCommentSet {
	return SchemaCommentSet{Tables: schemaComments}
}

// RegisteredTableNames 返回迁移注册模型对应的表名。
func RegisteredTableNames() []string {
	models := append([]any{}, coreModels()...)
	models = append(models, moderationModels()...)

	names := make([]string, 0, len(models))
	for _, m := range models {
		if named, ok := m.(interface{ TableName() string }); ok {
			names = append(names, named.TableName())
		}
	}
	return names
}

func quoteSQLComment(comment string) string {
	escaped := strings.ReplaceAll(comment, "'", "''")
	return "'" + escaped + "'"
}
