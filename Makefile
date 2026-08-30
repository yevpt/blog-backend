GO := $(shell which go || echo /Users/vpt/.g/go/bin/go)
SWAG := $(shell command -v swag 2>/dev/null || echo "$(GO) run github.com/swaggo/swag/cmd/swag@v1.16.6")
BINARY := bin/blog-server
MAIN := ./cmd/server
MIGRATE := ./cmd/migrate
SWAG_DIRS := $(MAIN),./internal/handler,./internal/dto,./pkg/response

.PHONY: run build dbsetup migrate-status migrate-adopt migrate-up swag test lint tidy clean hooks skills setup

# 克隆后执行一次：启用 git hooks + 同步 AI skill 符号链接
setup: hooks skills

# 启用 git hooks（commit message 校验）
hooks:
	git config core.hooksPath .githooks
	@chmod +x .githooks/* 2>/dev/null || true
	@echo "git hooks 已启用（core.hooksPath=.githooks）"

# 同步 .agents/skills/ → .claude/skills/ 符号链接（新增 skill 后执行）
skills:
	@sh scripts/sync-skills.sh

# 本地开发启动（需安装 air：go install github.com/air-verse/air@latest）
dev:
	air

# 直接运行（不热重载）
run:
	$(GO) run $(MAIN)

# 编译二进制
build:
	$(GO) build -o $(BINARY) $(MAIN)

# 初始化当前数据库结构和默认数据；默认管理员为 admin/admin
dbsetup:
	$(GO) run ./cmd/dbsetup

# 只读查看数据库迁移状态
migrate-status:
	$(GO) run $(MIGRATE) status

# 现有库首次接入台账；用法：make migrate-adopt THROUGH=20260704_admin_operation_log.sql
migrate-adopt:
	@test -n "$(THROUGH)" || (echo "请指定 THROUGH=<最后一个确认已执行的迁移.sql>"; exit 1)
	$(GO) run $(MIGRATE) adopt --through "$(THROUGH)"

# 显式执行尚未应用的版本化 SQL
migrate-up:
	$(GO) run $(MIGRATE) up

# 生成 swagger 文档；未安装 swag 时通过 go run 临时执行，避免依赖全局 PATH
swag:
	$(SWAG) init -g main.go -d $(SWAG_DIRS) -o docs

# 运行所有测试
test:
	$(GO) test ./... -v -cover

# 安装/整理依赖
tidy:
	$(GO) mod tidy

# 清理构建产物
clean:
	rm -rf bin/ tmp/ docs/swagger.json docs/swagger.yaml docs/docs.go
