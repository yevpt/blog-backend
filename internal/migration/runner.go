package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	defaultLockName    = "blog_backend_schema_migrations"
	defaultLockTimeout = 30
)

var (
	// ErrLedgerUninitialized 表示已有数据库尚未显式采纳历史迁移。
	ErrLedgerUninitialized = errors.New("迁移台账未初始化，请先对已有库执行 adopt；新库请使用 dbsetup")
	// ErrDirtyMigration 表示上次迁移发生了部分执行，必须人工核对后处理。
	ErrDirtyMigration = errors.New("存在 dirty 迁移，请人工核对数据库状态")
	// ErrChecksumMismatch 表示已登记的历史 SQL 被修改。
	ErrChecksumMismatch = errors.New("已执行迁移的校验和不一致")
	// ErrLockUnavailable 表示其他进程正在执行迁移。
	ErrLockUnavailable = errors.New("无法获取数据库迁移锁")
)

// State 表示迁移当前状态。
type State string

const (
	StateApplied State = "applied"
	StatePending State = "pending"
	StateDirty   State = "dirty"
)

// StatusEntry 是状态命令返回的单条迁移记录。
type StatusEntry struct {
	Version string
	State   State
}

// StatusReport 汇总台账是否存在以及全部迁移状态。
type StatusReport struct {
	Initialized bool
	Entries     []StatusEntry
}

// Runner 串行执行内嵌迁移并维护数据库台账。
type Runner struct {
	db          *sql.DB
	catalog     []Migration
	lockName    string
	lockTimeout int
}

// NewRunner 创建迁移执行器。
func NewRunner(db *sql.DB, catalog []Migration) *Runner {
	return &Runner{
		db: db, catalog: append([]Migration(nil), catalog...),
		lockName: defaultLockName, lockTimeout: defaultLockTimeout,
	}
}

// Status 只读检查台账和迁移状态；台账不存在时不会创建表。
func (r *Runner) Status(ctx context.Context) (StatusReport, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return StatusReport{}, fmt.Errorf("获取数据库连接: %w", err)
	}
	defer conn.Close()

	exists, err := ledgerExists(ctx, conn)
	if err != nil {
		return StatusReport{}, err
	}
	if !exists {
		return StatusReport{Entries: pendingEntries(r.catalog)}, nil
	}
	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return StatusReport{}, err
	}
	if len(applied) == 0 {
		return StatusReport{Entries: pendingEntries(r.catalog)}, nil
	}
	if err := validateApplied(r.catalog, applied); err != nil {
		return StatusReport{}, err
	}
	return StatusReport{Initialized: true, Entries: statusEntries(r.catalog, applied)}, nil
}

// Adopt 把 through 及之前的迁移登记为已执行，不运行 SQL。
func (r *Runner) Adopt(ctx context.Context, through string) (int, error) {
	target := migrationIndex(r.catalog, through)
	if target < 0 {
		return 0, fmt.Errorf("adopt 目标不存在: %s", through)
	}

	count := 0
	err := r.withLock(ctx, func(conn *sql.Conn) error {
		if err := ensureLedger(ctx, conn); err != nil {
			return err
		}
		applied, err := loadApplied(ctx, conn)
		if err != nil {
			return err
		}
		if err := validateApplied(r.catalog, applied); err != nil {
			return err
		}
		if err := ensureClean(applied); err != nil {
			return err
		}
		for _, item := range r.catalog[:target+1] {
			if _, exists := applied[item.Version]; exists {
				continue
			}
			if _, err := conn.ExecContext(ctx, adoptSQL, item.Version, item.Checksum); err != nil {
				return fmt.Errorf("登记迁移 %s: %w", item.Version, err)
			}
			count++
		}
		return nil
	})
	return count, err
}

// Up 按顺序执行全部未应用迁移。
func (r *Runner) Up(ctx context.Context) (int, error) {
	count := 0
	err := r.withLock(ctx, func(conn *sql.Conn) error {
		if err := ensureLedger(ctx, conn); err != nil {
			return err
		}
		applied, err := loadApplied(ctx, conn)
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			return ErrLedgerUninitialized
		}
		if err := validateApplied(r.catalog, applied); err != nil {
			return err
		}
		if err := ensureClean(applied); err != nil {
			return err
		}

		for _, item := range r.catalog {
			if _, exists := applied[item.Version]; exists {
				continue
			}
			if err := applyOne(ctx, conn, item); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func (r *Runner) withLock(ctx context.Context, run func(*sql.Conn) error) error {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("获取数据库连接: %w", err)
	}
	defer conn.Close()

	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", r.lockName, r.lockTimeout).Scan(&acquired); err != nil {
		return fmt.Errorf("获取数据库迁移锁: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return ErrLockUnavailable
	}
	defer releaseLock(conn, r.lockName)
	return run(conn)
}

func releaseLock(conn *sql.Conn, lockName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", lockName)
}

func applyOne(ctx context.Context, conn *sql.Conn, item Migration) error {
	if _, err := conn.ExecContext(ctx, beginMigrationSQL, item.Version, item.Checksum); err != nil {
		return fmt.Errorf("标记迁移 %s 为 dirty: %w", item.Version, err)
	}
	started := time.Now()
	if _, err := conn.ExecContext(ctx, item.SQL); err != nil {
		return fmt.Errorf("执行迁移 %s 失败，已保留 dirty 标记: %w", item.Version, err)
	}
	elapsed := time.Since(started).Milliseconds()
	result, err := conn.ExecContext(ctx, finishMigrationSQL, elapsed, item.Version)
	if err != nil {
		return fmt.Errorf("完成迁移 %s 台账: %w", item.Version, err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("完成迁移 %s 台账: 更新行数异常", item.Version)
	}
	return nil
}
