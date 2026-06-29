package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/vpt/blog-backend/internal/bootstrap"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/moderationmigration"
	"github.com/vpt/blog-backend/pkg/storage"
)

type options struct {
	BatchSize  int
	AfterType  string
	AfterID    uint64
	VerifyOnly bool
}

func main() {
	cfg := bootstrap.MustLoadConfig()
	opts, err := parseOptions(os.Args[1:], cfg.Moderation.Migration.BatchSize)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	logger := bootstrap.MustInitLogger(cfg)
	defer func() { _ = logger.Sync() }()
	db := bootstrap.MustInitMySQL(cfg)
	redisClient := bootstrap.MustInitRedis(cfg)
	store := bootstrap.MustInitStorage(cfg, redisClient)
	readable, ok := store.(interface {
		storage.ImageObjectReader
		storage.ObjectKeyResolver
	})
	if !ok {
		_, _ = fmt.Fprintln(os.Stderr, "对象存储不支持读取原图")
		os.Exit(1)
	}
	service := moderationmigration.NewService(moderationrepo.NewMigrationRepository(db), readable, moderationmigration.Options{
		RegistrationMode: moderationrepo.RegistrationMode(cfg.Moderation.Control.DefaultRegistrationMode),
		PublishingMode:   moderationrepo.PublishingMode(cfg.Moderation.Control.DefaultPublishingMode),
	})
	if opts.VerifyOnly {
		result, verifyErr := service.Verify(context.Background())
		writeJSON(result)
		if verifyErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, verifyErr)
			os.Exit(1)
		}
		return
	}

	cursor := moderationmigration.Cursor{Type: opts.AfterType, ID: opts.AfterID}
	for {
		result, runErr := service.RunBatch(context.Background(), cursor, opts.BatchSize)
		if runErr != nil {
			writeJSON(map[string]any{"next": cursor, "error": runErr.Error()})
			os.Exit(1)
		}
		writeJSON(result)
		if result.Done {
			return
		}
		cursor = result.Next
	}
}

func parseOptions(args []string, defaultBatchSize int) (options, error) {
	flags := flag.NewFlagSet("moderation-migrate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var result options
	flags.IntVar(&result.BatchSize, "batch-size", defaultBatchSize, "每个事务处理的记录数")
	flags.StringVar(&result.AfterType, "after-type", "", "续跑类型游标")
	flags.Uint64Var(&result.AfterID, "after-id", 0, "续跑业务 ID 游标")
	flags.BoolVar(&result.VerifyOnly, "verify-only", false, "只读校验迁移完整性")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if result.BatchSize <= 0 || result.BatchSize > 5000 {
		return options{}, fmt.Errorf("batch-size 必须在 1 到 5000 之间")
	}
	if result.AfterType == moderationmigration.DonePhase && result.AfterID != 0 {
		return options{}, fmt.Errorf("done 游标不能包含 after-id")
	}
	return result, nil
}

func writeJSON(value any) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}
