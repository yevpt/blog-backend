# Blog Backend API

个人博客的 Go API 服务（纯后端，前端为独立项目）。

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
| API 文档 | swaggo/swag → 导入 Apifox |

## 功能模块

- 文章管理（草稿/发布/分类/标签）
- 碎言（类 X 短贴）
- 评论系统（支持树形回复）
- 留言板
- 文件/媒体上传
- 用户管理（用户名/邮箱/手机 + bcrypt，预留微信登录）
- 友链管理

## 快速启动

### 本地开发

```bash
# 1. 复制本地配置并填写数据库信息
cp config/config.local.yaml.example config/config.local.yaml

# 2. 安装依赖
/Users/vpt/.g/go/bin/go mod tidy

# 3. 热重载启动（推荐）
# 先安装 air：go install github.com/air-verse/air@latest
air

# 或普通启动
make run
```

### 生产部署

```bash
# 1. 填写敏感变量
cp .env.example .env

# 2. 启动所有服务（MySQL + Redis + blog-server）
docker-compose up -d

# 日常更新
git pull && docker-compose build blog-server && docker-compose up -d --no-deps blog-server
```

## 项目结构

```
blog-backend/
├── cmd/server/main.go       # 程序入口
├── internal/
│   ├── handler/             # HTTP 层：接收请求，调用 service，返回响应
│   ├── service/             # 业务逻辑层
│   ├── repository/          # 数据访问层（GORM）
│   ├── model/               # 数据库模型（GORM struct）
│   ├── dto/                 # 请求/响应 DTO（与 model 解耦）
│   ├── middleware/          # Gin 中间件（鉴权、RBAC、日志、恢复）
│   └── router/              # 路由注册（全项目唯一入口）
├── pkg/
│   ├── config/              # 配置加载（Viper 多环境）
│   ├── database/            # MySQL 连接
│   ├── cache/               # Redis 连接
│   ├── storage/             # Garage 对象存储客户端
│   ├── jwt/                 # JWT 生成/解析
│   ├── response/            # 统一 API 响应格式
│   ├── roles/               # 角色常量和权限校验
│   └── logger/              # Zap 日志初始化
└── config/                  # 配置文件（YAML 多环境分层）
```

## 配置说明

配置按以下优先级叠加（高优先级覆盖低优先级）：

```
config.yaml          ← 公共基础（提交 git）
  ↓ 覆盖
config.{env}.yaml    ← 环境特定（dev/prod，提交 git）
  ↓ 覆盖
config.local.yaml    ← 本地密码（.gitignore，不提交）
  ↓ 覆盖
环境变量 BLOG_*      ← Docker 生产环境注入敏感值
```

通过 `APP_ENV` 环境变量切换：
```bash
APP_ENV=prod ./bin/blog-server
```

## 权限体系

三种角色，权重依次降低：

| 角色 | 标识 | 说明 |
|------|------|------|
| 管理员 | ROLE_ADMIN | 可访问所有接口 |
| VIP | ROLE_VIP | 可访问 VIP 及以下接口 |
| 普通用户 | ROLE_NORMAL | 默认角色 |

路由注册时通过中间件声明权限，类似 Spring 的 `@PreAuthorize`：

## 第三方登录

OAuth 认证身份只使用 `user`、`social_user`、`social_user_auth` 三张表；`user_social_link` 仅用于用户资料里的社交链接展示，不参与登录或绑定判断。

当前 Phase 1 已接入 GitHub：

```yaml
oauth:
  state_ttl_minutes: 10
  providers:
    github:
      enabled: true
      client_id: "your_github_client_id"
      client_secret: "your_github_client_secret"
      redirect_uri: "http://localhost:8080/oauth/github/callback"
```

本地 GitHub OAuth App 的 callback URL 需与 `redirect_uri` 精确一致。授权流程使用一次性 Redis state，并在支持的平台启用 PKCE；第三方 access token 只在后端保存，不返回前端。

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
make run        # 启动服务
make build      # 编译二进制到 bin/
make swag       # 生成 swagger 文档
make test       # 运行测试
make tidy       # 整理依赖
make clean      # 清理构建产物
```
