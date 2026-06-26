# Blog Backend API

个人博客的 Go API 服务（纯后端，前端为独立项目）。涵盖内容发布、社交化互动、站内通知与自建访问统计。

## 技术栈

| 用途   | 技术 |
|------|------|
| 语言   | Go 1.25+ |
| Web 框架 | Gin |
| ORM  | GORM + MySQL 8.4 |
| 日志   | Uber Zap |
| 配置   | Viper |
| 鉴权   | JWT（HS256）+ bcrypt 密码 |
| 缓存   | Redis |
| 对象存储 | Garage（S3 兼容） |
| 第三方登录 | golang.org/x/oauth2 + 项目内 Provider Adapter |
| 行为验证 | GoCaptcha |
| 地理解析 | ip2region（访问统计用，IP → 国家/省份/城市） |
| API 文档 | swaggo/swag → 导入 Apifox |

## 功能模块

- 文章管理（草稿/发布/分类/标签，可关联音乐）
- 碎言（类 X 短贴，支持图片与置顶）
- 评论系统（文章/碎言/留言板统一树形回复 + 点赞）
- 留言板
- 音乐管理（艺术家/专辑/歌曲，自动解析音频元数据）
- 文件/媒体上传（临时图片、头像、音乐资源，经 Garage + CDN 签名下发）
- 用户管理（用户名/邮箱/手机 + bcrypt，VIP/Admin 角色，预留微信登录）
- 友链管理
- 通知中心（站内信 + 邮件双通道，可配置接收偏好与发送配额）
- 站点访问统计（自建埋点，PV/UV/地理/设备/漏斗/实时在线）
- 第三方登录（GitHub/Gitee/QQ/微博/百度）
- 后台仪表盘（内容总量、近期互动、用户统计概览）

## 快速启动

### 本地开发

```bash
# 1. 复制本地配置并填写数据库信息
cp config/config.local.yaml.example config/config.local.yaml

# 2. 安装依赖
go mod tidy

# 3. 初始化仓库本地设置（启用 git hooks + 同步 AI skill 链接）—— 克隆后执行一次
make setup

# 4. 初始化数据库结构和默认数据
make dbsetup

# 5. 热重载启动（推荐，需先安装 air：go install github.com/air-verse/air@latest）
make dev

# 或不热重载直接运行
make run
```

### 生产部署

GitHub Actions（`.github/workflows/deploy.yml`）负责跑测试、构建镜像并推送到腾讯云镜像仓库，再通过 SSH 远程执行 `docker compose` 完成部署；服务器只负责拉取运行，MySQL / Redis 由宝塔管理，容器内只跑 `blog-server`。

| 分支 | 环境 | 容器名 | 配置覆盖 |
|------|------|------|------|
| `dev`  | staging（测试） | `blog-server-test` | `config.test.yaml` |
| `main` | production（生产） | `blog-server`      | `config.prod.yaml` |

服务器部署目录（`DEPLOY_DIR`，如 `/root/docker/blog-backend`）为扁平结构：

```
config/             # config.yaml + config.{prod,test}.yaml（CI 自动同步）
geoip/              # ip2region_v4.xdb / ip2region_v6.xdb（启用地理统计时手动放置）
docker-compose.yml  # CI 自动同步
.env                # 密钥，首次手动创建（见 .env.example）
```

首次部署只需在服务器创建 `.env`：

```bash
# 在 DEPLOY_DIR 下
cp .env.example .env   # 然后填写真实密钥
```

之后推送到 `dev` / `main` 由 CI 自动部署对应环境（按 commit SHA 拉取精确镜像）。
需要手动起停时，进入 DEPLOY_DIR 一行搞定：

```bash
docker compose up -d   # 同目录 .env 自动加载，镜像默认回退到 :latest
```

## 项目结构

```
blog-backend/
├── cmd/
│   ├── server/main.go       # API 服务入口
│   └── dbsetup/main.go      # 当前数据库初始化入口
├── internal/
│   ├── bootstrap/           # 启动期依赖组装（config/log/db/redis/jwt/mailer/storage/worker）
│   ├── dbschema/            # 当前表结构注册和默认种子数据
│   ├── handler/             # HTTP 层：接收请求，调用 service，返回响应
│   ├── service/             # 业务逻辑层
│   ├── repository/          # 数据访问层（GORM）
│   ├── worker/              # 后台异步任务（通知分发/邮件发送，统计落库/日聚合）
│   ├── oauth/               # 第三方登录：State 管理、Provider 适配
│   ├── model/               # 数据库模型（GORM struct）
│   ├── dto/                 # 请求/响应 DTO（与 model 解耦）
│   ├── middleware/          # Gin 中间件（鉴权、RBAC、限流、日志、恢复）
│   └── router/              # 路由注册（全项目唯一入口）
├── pkg/
│   ├── config/              # 配置加载（Viper 多环境）
│   ├── database/            # MySQL 连接
│   ├── cache/                # Redis 连接
│   ├── storage/             # Garage 对象存储客户端
│   ├── email/               # SMTP 邮件发送
│   ├── imagefile/imageutil/audiofile/  # 图片/音频文件校验与处理
│   ├── jwt/                 # JWT 生成/解析
│   ├── response/            # 统一 API 响应格式
│   ├── roles/               # 角色常量和权限校验
│   ├── strutil/             # 字符串工具
│   └── logger/              # Zap 日志初始化
└── config/                  # 配置文件（YAML 多环境分层）
```

## 数据库初始化

首次部署新库时执行：

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

未来增量 SQL 只从 `migrations/20260625_baseline.sql` 之后开始添加，初始化命令和历史数据迁移保持分离。

已有库升级时，按顺序手动执行 `migrations/` 下尚未应用的 SQL。例如 `20260626_user_last_active_at.sql` 新增 `last_active_at` 并从 `last_login_at` 回填（UPDATE 段可重复执行）。

## 配置说明

配置按以下优先级叠加（高优先级覆盖低优先级）：

```
config.yaml          ← 公共基础（提交 git）
  ↓ 覆盖
config.{env}.yaml    ← 环境特定（dev/test/prod，提交 git）
  ↓ 覆盖
config.local.yaml    ← 本地密码（.gitignore，不提交）
  ↓ 覆盖
环境变量 BLOG_*      ← Docker 生产环境注入敏感值
```

通过 `APP_ENV` 环境变量切换：
```bash
APP_ENV=prod ./bin/blog-server
```

### 专项配置文档

- [站点分析（Analytics）配置与部署](docs/analytics-configuration.md) —— 采集/聚合/公开接口的全部配置项、前后端跨仓密钥耦合、`site_host` 与 `ANALYTICS_ALLOWED_ORIGINS` 区别、随机串生成、迁移与时钟等。

## 权限体系

三种角色，权重依次降低：

| 角色 | 标识 | 说明 |
|------|------|------|
| 管理员 | ROLE_ADMIN | 可访问所有接口 |
| VIP | ROLE_VIP | 可访问 VIP 及以下接口 |
| 普通用户 | ROLE_NORMAL | 默认角色 |

路由注册时通过中间件声明权限，类似 Spring 的 `@PreAuthorize`：

```go
// 公开（无需登录）
r.GET("/articles", ...)

// 需要登录
authed := r.Group("/", middleware.Auth(jwtMgr))

// 需要 VIP 权限
vip := r.Group("/", middleware.Auth(jwtMgr), middleware.RequireRole(roles.VipRole))

// 仅管理员
admin := r.Group("/admin", middleware.Auth(jwtMgr), middleware.RequireRole(roles.AdminRole))
```

## 限流与封禁

所有限流统一实现在 `internal/middleware/ratelimit.go`，按 IP 或登录用户 ID（principal）维度用 Redis 做滑动窗口计数：超过软限只返回 `429` 提示减速，超过硬限则触发封禁。

| 档位 | 适用场景 | 窗口 / 软限 / 硬限 | 命中硬限后递进封禁时长（独立计数，过期自动清零） |
|------|------|------|------|
| Strict | 发送验证码、注册、密码重置等高风险接口 | 60s / 5 / 20 | 10min → 30min → 2h → 24h（24h 内累计违规次数） |
| Normal | 后台敏感操作（如 analytics backfill） | 60s / 10 / 30 | 10min → 30min → 2h → 24h（24h 内累计违规次数） |
| Loose | 登录、管理员登录、OAuth 授权/回调 | 120s / 30 / 100 | 10min → 30min → 2h → 24h（24h 内累计违规次数） |
| Public | 公开 GET 接口兜底防护（文章/分类/音乐/友链等几乎全部公开接口） | 300s / 5000 / 20000 | 5min → 30min → 2h → 24h → 48h → 7天（7天内累计违规次数） |
| 上传类（碎语/临时图片/头像） | 按登录用户 ID 限制上传频率（未登录回退到 IP），管理员阈值更宽 | 各接口独立配置 | 与 Strict 同曲线：10min → 30min → 2h → 24h |

设计要点：

- **按档位隔离**：每个档位的封禁 key 独立命名（如 `ban:strict:ip:<IP>`、`ban:public:ip:<IP>`），互不连带——刷公开接口被封不影响登录，登录被封也不影响访问公开内容。
- **递进式封禁**：同一档位在窗口期内（24h，Public 为 7 天）反复触发硬限会逐级延长封禁时长；期间无新增违规，计数会自动清零，不存在"永久封禁"。
- **并发安全**：高并发下若多个请求同时冲过硬限，只有第一个请求负责升级计数与落键封禁，避免一次性突发流量被误判成多次违规直接跳档重罚。
- **Redis 故障安全退化**：限流依赖的 Redis 命令出错时不会抛异常导致 500，会安全退化为最低档处理；Redis 不可用期间限流整体失效开放（fail open），不会误拦正常请求。
- **暂无后台解封接口**：误封后需运维直连 Redis 执行 `DEL ban:<档位>:ip:<IP>`（如该 IP 已有递进计数，再补删 `ban:<档位>:ip:<IP>:count`）手动解封；封禁本身到期会自动失效，无需人工干预。

## 第三方登录

OAuth 认证身份只使用 `user`、`social_user`、`social_user_auth` 三张表；`user_social_link` 仅用于用户资料里的社交链接展示，不参与登录或绑定判断。

当前已接入 GitHub、Gitee、QQ、微博、百度：

```yaml
oauth:
  state_ttl_minutes: 10
  providers:
    github:
      enabled: true
      client_id: "your_github_client_id"
      client_secret: "your_github_client_secret"
      redirect_uri: "http://localhost:8080/oauth/github/callback"
    gitee:
      enabled: false
      client_id: "your_gitee_client_id"
      client_secret: "your_gitee_client_secret"
      redirect_uri: "http://localhost:8080/oauth/gitee/callback"
    qq:
      enabled: false
      client_id: "your_qq_client_id"
      client_secret: "your_qq_client_secret"
      redirect_uri: "http://localhost:8080/oauth/qq/callback"
    weibo:
      enabled: false
      client_id: "your_weibo_client_id"
      client_secret: "your_weibo_client_secret"
      redirect_uri: "http://localhost:8080/oauth/weibo/callback"
    baidu:
      enabled: false
      client_id: "your_baidu_client_id"
      client_secret: "your_baidu_client_secret"
      redirect_uri: "http://localhost:8080/oauth/baidu/callback"
```

本地 GitHub OAuth App 的 callback URL 需与 `redirect_uri` 精确一致。授权流程使用一次性 Redis state，并在支持的平台启用 PKCE；第三方 access token 只在后端保存，不返回前端。

## 通知与统计

### 通知中心

通知拆分为站内信与邮件两条独立链路，由后台 worker（`internal/worker/notification`）异步驱动，三条处理链各自独立运行：

- **Dispatch**（5s 一轮）：业务事件（评论回复、点赞等）经 Publisher 发布后，由 Dispatcher 解析收件人与偏好，写入站内信收件箱。
- **Plan**（30s 一轮）：按全站 / 用途 / 角色 / 接收人四层配额评估邮件发送额度，规划待发批次。
- **Send**（按配置间隔）：从待发批次中取出并通过 SMTP 实际发送，失败任务进入批次可在后台重试。

站内信走 `GET/PATCH/DELETE /notifications*` 系列接口；邮件任务、批次与配额由 `/admin/notifications/*` 管理。`email.worker_enabled=false` 时只关闭邮件链路，站内信不受影响。

### 站点访问统计

自建埋点系统（类 GA/百度统计）：前端 `/collect` 上报 → Enricher 富化（UA 解析 + ip2region 地理解析）→ Redis 实时层（今日/在线）与异步落库双写 → 每日定时 Rollup 聚合 + 过期清理。后台可查概览/趋势/页面/维度/漏斗/实时等维度，前台暴露精简的公开统计接口。完整配置项、最小可用模式与跨仓耦合细节见[站点分析配置文档](docs/analytics-configuration.md)。

## 测试接口

骨架阶段提供以下测试接口，用于验证框架和权限是否正常：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /health | 检查 DB/Redis 连通状态 |
| GET | /test/public | 公开接口测试 |
| POST | /test/token | 生成测试 JWT（仅非生产环境） |
| GET | /test/authed | 需要 JWT |
| GET | /test/vip | 需要 VIP 权限 |
| GET | /admin/test | 需要 Admin 权限 |

## 常用命令

```bash
make dev        # 热重载启动（等价于直接执行 air，需先安装 air）
make run        # 直接运行（不热重载）
make build      # 编译二进制到 bin/
make dbsetup    # 初始化当前数据库结构和默认数据
make swag       # 生成 swagger 文档
make test       # 运行测试
make tidy       # 整理依赖
make clean      # 清理构建产物
make setup      # 克隆后初始化：启用 git hooks + 同步 skill 链接
make hooks      # 仅启用 git hooks（commit message 校验）
make skills     # 仅同步 .agents/skills → .claude/skills 符号链接
```

> 注意：`make hooks` 通过 `git config core.hooksPath .githooks` 启用，属于**仓库本地配置、不随提交同步**，因此**每次克隆后需执行一次** `make setup`。commit message 规范见 `.agents/skills/git-commit/SKILL.md`。
