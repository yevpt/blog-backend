// Package migrations 内嵌版本化 SQL，供迁移命令和新库初始化记录复用。
package migrations

import "embed"

// Files 包含仓库中全部版本化 SQL。
//
//go:embed *.sql
var Files embed.FS
