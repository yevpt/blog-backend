// Package migration 提供版本化 SQL 的发现、校验和执行能力。
package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

var migrationNamePattern = regexp.MustCompile(`^[0-9]{8}_[a-z0-9_]+\.sql$`)

// Migration 是一份不可变的版本化 SQL。
type Migration struct {
	Version  string
	SQL      string
	Checksum string
}

// LoadCatalog 从内嵌文件系统加载并按文件名排序迁移。
func LoadCatalog(files fs.FS) ([]Migration, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("扫描迁移文件: %w", err)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("未找到迁移 SQL")
	}

	result := make([]Migration, 0, len(names))
	for _, name := range names {
		version := path.Base(name)
		if !migrationNamePattern.MatchString(version) {
			return nil, fmt.Errorf("迁移文件名不合法: %s", version)
		}
		content, readErr := fs.ReadFile(files, name)
		if readErr != nil {
			return nil, fmt.Errorf("读取迁移 %s: %w", version, readErr)
		}
		if strings.TrimSpace(string(content)) == "" {
			return nil, fmt.Errorf("迁移内容为空: %s", version)
		}
		sum := sha256.Sum256(content)
		result = append(result, Migration{
			Version: version, SQL: string(content), Checksum: hex.EncodeToString(sum[:]),
		})
	}
	return result, nil
}

func migrationIndex(catalog []Migration, version string) int {
	for index := range catalog {
		if catalog[index].Version == version {
			return index
		}
	}
	return -1
}
