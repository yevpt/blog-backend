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

该命令负责当前版本的一键建表、默认数据和迁移台账初始化，不会重放用于历史旧库的增量 SQL。默认会创建 `id=1` 的管理员用户：

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

- `cmd/dbsetup` 面向新库或开发库初始化，并把当前全部内嵌迁移登记为已采纳。
- `cmd/migrate` 面向已有库，按文件名顺序执行增量 SQL，并校验历史文件 SHA-256。
- 服务启动不会自动建表或迁移；迁移必须在部署阶段显式执行。
- 历史 UGC 接入内容审核时，使用 `cmd/moderation-migrate` 分批迁移，详细顺序见 [内容审核上线与回滚](moderation-rollout.md)。
- 未来增量 SQL 从 `migrations/20260625_baseline.sql` 之后继续添加，初始化命令和历史数据迁移保持分离。

已有库首次接入台账时，先确认最后一个已经人工执行的 SQL，只登记到该文件。不要为了消除 pending 状态而登记尚未执行的迁移：

```bash
make migrate-status
make migrate-adopt THROUGH=20260704_admin_operation_log.sql
make migrate-up
```

迁移执行前会获取 MySQL advisory lock；执行失败会保留 dirty 标记并停止。MySQL DDL 不能保证事务回滚，出现 dirty 后应先从备份恢复或人工核对表结构，不要直接修改台账继续运行。

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
make migrate-status # 查看迁移状态
make migrate-adopt THROUGH=<file.sql> # 已有库首次登记历史迁移
make migrate-up # 执行全部待应用迁移
make swag       # 生成 Swagger 文档
make test       # 运行所有测试
make tidy       # 整理依赖
make clean      # 清理构建产物
make setup      # 克隆后初始化 git hooks 与 skill 链接
make hooks      # 仅启用 git hooks
make skills     # 仅同步 .agents/skills
```

`make hooks` 通过 `git config core.hooksPath .githooks` 启用，属于仓库本地配置，不随提交同步。
