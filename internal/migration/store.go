package migration

import (
	"context"
	"database/sql"
	"fmt"
)

const createLedgerSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
  version varchar(191) NOT NULL,
  checksum char(64) NOT NULL,
  dirty tinyint(1) NOT NULL DEFAULT 0,
  applied_at datetime(6) NULL,
  execution_ms bigint unsigned NOT NULL DEFAULT 0,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='数据库迁移台账'`

const (
	adoptSQL           = "INSERT INTO schema_migrations (version, checksum, dirty, applied_at, execution_ms) VALUES (?, ?, 0, CURRENT_TIMESTAMP(6), 0)"
	beginMigrationSQL  = "INSERT INTO schema_migrations (version, checksum, dirty, applied_at, execution_ms) VALUES (?, ?, 1, NULL, 0)"
	finishMigrationSQL = "UPDATE schema_migrations SET dirty = 0, applied_at = CURRENT_TIMESTAMP(6), execution_ms = ? " +
		"WHERE version = ? AND dirty = 1"
)

type appliedMigration struct {
	Checksum string
	Dirty    bool
}

func ledgerExists(ctx context.Context, conn *sql.Conn) (bool, error) {
	var count int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = 'schema_migrations'`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("检查迁移台账: %w", err)
	}
	return count > 0, nil
}

func ensureLedger(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, createLedgerSQL); err != nil {
		return fmt.Errorf("创建迁移台账: %w", err)
	}
	return nil
}

func loadApplied(ctx context.Context, conn *sql.Conn) (map[string]appliedMigration, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version, checksum, dirty FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("读取迁移台账: %w", err)
	}
	defer rows.Close()

	result := make(map[string]appliedMigration)
	for rows.Next() {
		var version string
		var checksum string
		var dirty bool
		if err := rows.Scan(&version, &checksum, &dirty); err != nil {
			return nil, fmt.Errorf("解析迁移台账: %w", err)
		}
		result[version] = appliedMigration{Checksum: checksum, Dirty: dirty}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取迁移台账: %w", err)
	}
	return result, nil
}

func validateApplied(catalog []Migration, applied map[string]appliedMigration) error {
	known := make(map[string]Migration, len(catalog))
	for _, item := range catalog {
		known[item.Version] = item
	}
	for version := range applied {
		if _, exists := known[version]; !exists {
			return fmt.Errorf("迁移台账包含未知版本: %s", version)
		}
	}

	seenPending := false
	for _, item := range catalog {
		record, exists := applied[item.Version]
		if !exists {
			seenPending = true
			continue
		}
		if seenPending {
			return fmt.Errorf("迁移台账顺序不连续，已应用版本前存在缺口: %s", item.Version)
		}
		if record.Checksum != item.Checksum {
			return fmt.Errorf("%w: %s", ErrChecksumMismatch, item.Version)
		}
	}
	return nil
}

func ensureClean(applied map[string]appliedMigration) error {
	for version, record := range applied {
		if record.Dirty {
			return fmt.Errorf("%w: %s", ErrDirtyMigration, version)
		}
	}
	return nil
}

func pendingEntries(catalog []Migration) []StatusEntry {
	entries := make([]StatusEntry, 0, len(catalog))
	for _, item := range catalog {
		entries = append(entries, StatusEntry{Version: item.Version, State: StatePending})
	}
	return entries
}

func statusEntries(catalog []Migration, applied map[string]appliedMigration) []StatusEntry {
	entries := make([]StatusEntry, 0, len(catalog))
	for _, item := range catalog {
		state := StatePending
		if record, exists := applied[item.Version]; exists {
			state = StateApplied
			if record.Dirty {
				state = StateDirty
			}
		}
		entries = append(entries, StatusEntry{Version: item.Version, State: state})
	}
	return entries
}
