# 开发指南

本文记录本地开发、数据库初始化、迁移和测试入口。项目根 README 只保留最短启动路径。

## 本地启动

```bash
cp config/config.local.yaml.example config/config.local.yaml
go mod tidy
make setup
make dbsetup
make dev
```

`make dev` 依赖 Air：

```bash
go install github.com/air-verse/air@latest
```

未安装 Air 时使用：

```bash
make run
```

## 数据库初始化

新库首次初始化：

```bash
make dbsetup
```

该命令只负责当前版本的一键建表和默认数据，不包含历史旧库迁移。默认会创建 `id=1` 的管理员用户：

| 字段 | 默认值 |
|------|------|
| 用户 ID | `1` |
| 用户名 | `admin` |
| 初始密码 | `admin` |
| 角色 | `ROLE_ADMIN` |

可选覆盖：

```bash
BLOG_DBSETUP_ADMIN_USERNAME='admin' \
BLOG_DBSETUP_ADMIN_PASSWORD='admin' \
BLOG_DBSETUP_ADMIN_EMAIL='admin@example.com' \
make dbsetup
```

## 迁移策略

- `cmd/dbsetup` 面向新库或开发库初始化。
- 现有库升级时，按顺序手动执行 `migrations/` 下尚未应用的 SQL。
- 历史 UGC 接入内容审核时，使用 `cmd/moderation-migrate` 分批迁移，详细顺序见 [内容审核上线与回滚](moderation-rollout.md)。
- 未来增量 SQL 从 `migrations/20260625_baseline.sql` 之后继续添加，初始化命令和历史数据迁移保持分离。

示例：

```bash
mysql < migrations/20260626_user_last_active_at.sql
go run ./cmd/moderation-migrate --verify-only
```

## 测试接口

骨架阶段保留以下测试接口，用于验证框架和权限是否正常：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 检查 DB/Redis 连通状态 |
| GET | `/test/public` | 公开接口测试 |
| POST | `/test/token` | 生成测试 JWT，仅非生产环境 |
| GET | `/test/authed` | 需要 JWT |
| GET | `/test/vip` | 需要 VIP 权限 |
| GET | `/admin/test` | 需要 Admin 权限 |

## 常用命令

```bash
make dev        # 热重载启动
make run        # 直接运行
make build      # 编译二进制
make dbsetup    # 初始化当前数据库结构和默认数据
make swag       # 生成 Swagger 文档
make test       # 运行所有测试
make tidy       # 整理依赖
make clean      # 清理构建产物
make setup      # 克隆后初始化 git hooks 与 skill 链接
make hooks      # 仅启用 git hooks
make skills     # 仅同步 .agents/skills
```

`make hooks` 通过 `git config core.hooksPath .githooks` 启用，属于仓库本地配置，不随提交同步。
