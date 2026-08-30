# Blog Backend API

个人博客 Go API 服务，前端为独立项目。项目覆盖内容发布、用户与社交登录、评论互动、站内通知、邮件通知、内容审核、自建访问统计、媒体上传与 CDN 私有访问。

## 项目要点

- **分层架构**：`handler -> service -> repository` 单向依赖，基础设施由 `internal/bootstrap` 构造后注入。
- **统一响应与 DTO**：HTTP 返回走 `pkg/response`，前端契约使用 `internal/dto`，不直接暴露 `model.*`。
- **多环境配置**：`config.yaml`、`config.{env}.yaml`、`config.local.yaml` 与 `BLOG_*` 环境变量分层覆盖。
- **媒体链路**：Garage S3 兼容存储，支持 CDN 私有签名 URL、图片处理、头像归一化、音乐资源上传。
- **异步 worker**：通知分发、邮件规划/发送、统计落库/聚合、审核清理、规则构建等后台任务随服务启动。
- **安全边界**：JWT + RBAC、Redis 滑动窗口限流、OAuth state/PKCE、图形验证码、审核治理与操作日志。
- **API 文档**：Swagger 由 `make swag` 生成到 `docs/`，可导入 Apifox。

## 技术栈

| 用途 | 技术 |
|------|------|
| 语言 | Go 1.25+ |
| Web 框架 | Gin |
| ORM / DB | GORM + MySQL 8.4 |
| 缓存 / 限流 | Redis |
| 日志 | Uber Zap |
| 配置 | Viper |
| 鉴权 | JWT（HS256）+ bcrypt |
| 对象存储 | Garage（S3 兼容）+ CDN 签名 |
| 第三方登录 | golang.org/x/oauth2 + Provider Adapter |
| 行为验证 | GoCaptcha |
| 地理解析 | ip2region |
| API 文档 | swaggo/swag |

## 快速启动

```bash
# 1. 复制本地配置并填写 MySQL / Redis / JWT / Garage 等信息
cp config/config.local.yaml.example config/config.local.yaml

# 2. 安装依赖
go mod tidy

# 3. 克隆后执行一次：启用 git hooks + 同步 AI skill 链接
make setup

# 4. 初始化当前库表结构和默认数据
make dbsetup

# 5. 启动开发服务
make dev
```

未安装 Air 时可用 `make run` 直接启动。默认管理员由 `make dbsetup` 创建，用户名/密码为 `admin/admin`，可通过 `BLOG_DBSETUP_ADMIN_*` 环境变量覆盖。

## 项目结构

```text
blog-backend/
├── cmd/
│   ├── server/              # API 服务入口
│   ├── dbsetup/             # 当前版本新库初始化
│   ├── migrate/             # 生产增量 SQL 台账与执行入口
│   └── moderation-migrate/  # 历史 UGC 审核数据迁移与校验
├── internal/
│   ├── bootstrap/           # config/log/db/redis/jwt/mailer/storage/worker 组装
│   ├── router/              # 路由注册唯一入口
│   ├── handler/             # HTTP 层
│   ├── service/             # 业务逻辑层
│   ├── repository/          # GORM 数据访问层
│   ├── worker/              # 后台异步任务
│   ├── oauth/               # 第三方登录流程与 Provider 适配
│   ├── model/               # GORM 模型
│   ├── dto/                 # 请求/响应 DTO
│   ├── middleware/          # 鉴权、RBAC、限流、日志、恢复等中间件
│   └── dbschema/            # 当前表结构注册与种子数据
├── pkg/                     # 可复用基础设施与工具包
├── config/                  # 多环境 YAML 配置
├── migrations/              # 版本化增量 SQL
├── docs/                    # Swagger 产物与专题文档
└── docker/                  # 镜像构建文件
```

## 文档入口

| 文档 | 内容 |
|------|------|
| [开发指南](docs/development.md) | 本地启动、数据库初始化、迁移策略、常用命令、测试接口 |
| [部署与配置](docs/deployment.md) | 配置覆盖顺序、生产部署目录、GitHub Actions 部署流程、环境变量 |
| [功能地图](docs/features.md) | 文章、碎语、评论、用户、OAuth、通知、统计、审核、媒体等模块概览 |
| [运行时与安全](docs/runtime-operations.md) | 权限体系、限流封禁、通知 worker、站点统计运行链路 |
| [站点分析配置](docs/analytics-configuration.md) | Analytics 全量配置、跨仓密钥、采集令牌、GeoIP 与部署注意事项 |
| [内容审核上线与回滚](docs/moderation-rollout.md) | 审核迁移、启用顺序、回滚方案、规则管理上线 |
| [审核规则索引基准](docs/moderation-rule-index-benchmark.md) | 规则索引构建与匹配性能基准 |
| [对象存储说明](pkg/storage/README.md) | Garage/CDN 签名 URL、缓存、接口约定 |

## 常用命令

```bash
make dev        # 热重载启动，需先安装 air
make run        # 直接运行 API 服务
make build      # 编译到 bin/blog-server
make dbsetup    # 初始化当前版本新库结构和默认数据
make migrate-status # 查看版本化 SQL 状态
make migrate-up # 执行待应用迁移
make swag       # 生成 Swagger 文档到 docs/
make test       # 运行全部测试
make tidy       # 整理依赖
make clean      # 清理构建产物和 Swagger 产物
make setup      # 启用 git hooks + 同步 .agents/skills
make hooks      # 仅启用 commit message hook
make skills     # 同步 .agents/skills -> .claude/skills
```

`make hooks` 会写入仓库本地 `core.hooksPath=.githooks`，克隆后需执行一次 `make setup`。commit message 规范见 `.agents/skills/git-commit/SKILL.md`。

## API 文档

```bash
make swag
```

Swagger 产物生成到 `docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml`。业务接口以 `internal/router` 的公开、登录、VIP、Admin 分组为准。
