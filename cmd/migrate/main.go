// 数据库版本迁移工具：显式采纳已有迁移或执行尚未应用的内嵌 SQL。
//
// 用法：
//
//	blog-migrate status
//	blog-migrate adopt --through 20260704_admin_operation_log.sql
//	blog-migrate up
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/vpt/blog-backend/internal/migration"
	"github.com/vpt/blog-backend/migrations"
	"github.com/vpt/blog-backend/pkg/config"
)

type migrateOptions struct {
	Action  string
	Through string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseMigrateOptions(args)
	if err != nil {
		return err
	}
	catalog, err := migration.LoadCatalog(migrations.Files)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}

	db, err := sql.Open("mysql", cfg.DB.DSN()+"&multiStatements=true")
	if err != nil {
		return fmt.Errorf("创建数据库连接: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("连接数据库: %w", err)
	}

	runner := migration.NewRunner(db, catalog)
	switch opts.Action {
	case "status":
		return printStatus(ctx, output, runner)
	case "adopt":
		count, adoptErr := runner.Adopt(ctx, opts.Through)
		if adoptErr != nil {
			return adoptErr
		}
		_, _ = fmt.Fprintf(output, "已登记 %d 个历史迁移\n", count)
		return nil
	case "up":
		count, upErr := runner.Up(ctx)
		if upErr != nil {
			return upErr
		}
		_, _ = fmt.Fprintf(output, "已执行 %d 个迁移\n", count)
		return nil
	default:
		return fmt.Errorf("不支持的迁移动作: %s", opts.Action)
	}
}

func parseMigrateOptions(args []string) (migrateOptions, error) {
	if len(args) == 0 {
		return migrateOptions{}, fmt.Errorf("缺少命令，可用命令: status、adopt、up")
	}
	opts := migrateOptions{Action: args[0]}
	fs := flag.NewFlagSet("blog-migrate "+opts.Action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if opts.Action == "adopt" {
		fs.StringVar(&opts.Through, "through", "", "最后一个确认已执行的迁移文件名")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return migrateOptions{}, err
	}
	if fs.NArg() != 0 {
		return migrateOptions{}, fmt.Errorf("存在多余参数: %v", fs.Args())
	}

	switch opts.Action {
	case "status", "up":
		return opts, nil
	case "adopt":
		if opts.Through == "" {
			return migrateOptions{}, fmt.Errorf("adopt 必须指定 --through")
		}
		return opts, nil
	default:
		return migrateOptions{}, fmt.Errorf("未知命令 %q，可用命令: status、adopt、up", opts.Action)
	}
}

func printStatus(ctx context.Context, output io.Writer, runner *migration.Runner) error {
	report, err := runner.Status(ctx)
	if err != nil {
		return err
	}
	if !report.Initialized {
		_, _ = fmt.Fprintln(output, "迁移台账未初始化；已有库请先执行 adopt，新库请使用 dbsetup")
	}
	for _, entry := range report.Entries {
		_, _ = fmt.Fprintf(output, "%s\t%s\n", entry.State, entry.Version)
	}
	return nil
}
