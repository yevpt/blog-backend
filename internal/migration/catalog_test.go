package migration_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/migration"
	migrationfiles "github.com/vpt/blog-backend/migrations"
)

func TestLoadCatalog_SortsAndChecksumsMigrations(t *testing.T) {
	files := fstest.MapFS{
		"20260102_second.sql": {Data: []byte("ALTER TABLE users ADD COLUMN nickname varchar(64);")},
		"20260101_first.sql":  {Data: []byte("CREATE TABLE users (id bigint PRIMARY KEY);")},
	}

	catalog, err := migration.LoadCatalog(files)

	require.NoError(t, err)
	require.Len(t, catalog, 2)
	assert.Equal(t, "20260101_first.sql", catalog[0].Version)
	assert.Equal(t, checksum(files["20260101_first.sql"].Data), catalog[0].Checksum)
	assert.Equal(t, string(files["20260101_first.sql"].Data), catalog[0].SQL)
	assert.Equal(t, "20260102_second.sql", catalog[1].Version)
}

func TestLoadCatalog_RejectsInvalidFilename(t *testing.T) {
	files := fstest.MapFS{"001-init.sql": {Data: []byte("SELECT 1;")}}

	_, err := migration.LoadCatalog(files)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "迁移文件名不合法")
}

func TestLoadCatalog_RejectsEmptyMigration(t *testing.T) {
	files := fstest.MapFS{"20260101_empty.sql": {Data: []byte(" \n\t")}}

	_, err := migration.LoadCatalog(files)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "迁移内容为空")
}

func TestLoadCatalog_LoadsEmbeddedProjectMigrations(t *testing.T) {
	catalog, err := migration.LoadCatalog(migrationfiles.Files)

	require.NoError(t, err)
	assert.Greater(t, len(catalog), 1)
	assert.Equal(t, "20260625_baseline.sql", catalog[0].Version)
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
