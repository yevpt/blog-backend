GO := $(shell which go || echo /Users/vpt/.g/go/bin/go)
SWAG := $(shell command -v swag 2>/dev/null || echo "$(GO) run github.com/swaggo/swag/cmd/swag@v1.16.6")
BINARY := bin/blog-server
MAIN := ./cmd/server
SWAG_DIRS := $(MAIN),./internal/handler,./internal/dto,./pkg/response

.PHONY: run build swag test lint tidy clean hooks

# 启用 git hooks（commit message 校验）；克隆后执行一次
hooks:
	git config core.hooksPath .githooks
	@chmod +x .githooks/* 2>/dev/null || true
	@echo "git hooks 已启用（core.hooksPath=.githooks）"

# 本地开发启动（需安装 air：go install github.com/air-verse/air@latest）
dev:
	air

# 直接运行（不热重载）
run:
	$(GO) run $(MAIN)

# 编译二进制
build:
	$(GO) build -o $(BINARY) $(MAIN)

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
