GO := $(shell which go || echo /Users/vpt/.g/go/bin/go)
BINARY := bin/blog-server
MAIN := ./cmd/server

.PHONY: run build swag test lint tidy clean

# 本地开发启动（需安装 air：go install github.com/air-verse/air@latest）
dev:
	air

# 直接运行（不热重载）
run:
	$(GO) run $(MAIN)

# 编译二进制
build:
	$(GO) build -o $(BINARY) $(MAIN)

# 生成 swagger 文档（需安装 swag：go install github.com/swaggo/swag/cmd/swag@latest）
swag:
	swag init -g $(MAIN)/main.go -o docs

# 运行所有测试
test:
	$(GO) test ./... -v -cover

# 安装/整理依赖
tidy:
	$(GO) mod tidy

# 清理构建产物
clean:
	rm -rf bin/ tmp/ docs/swagger.json docs/swagger.yaml docs/docs.go
