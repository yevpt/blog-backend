# 部署与配置

本文记录配置覆盖顺序、生产目录结构和 CI 部署流程。

## 配置覆盖顺序

配置按以下优先级叠加，高优先级覆盖低优先级：

```text
config.yaml          <- 公共基础，提交 git
  ↓ 覆盖
config.{env}.yaml    <- 环境特定，dev/test/prod，提交 git
  ↓ 覆盖
config.local.yaml    <- 本地密码，.gitignore，不提交
  ↓ 覆盖
环境变量 BLOG_*      <- Docker/生产环境注入敏感值
```

通过 `APP_ENV` 切换环境：

```bash
APP_ENV=prod ./bin/blog-server
```

生产敏感项必须通过环境变量或未提交的配置注入，重点包括：

- `BLOG_JWT_SECRET`
- `BLOG_DB_*`
- `BLOG_REDIS_*`
- `BLOG_GARAGE_*`
- `BLOG_CDN_*`
- `BLOG_IMAGE_ORIGINAUTHSECRET`
- `BLOG_EMAIL_*`
- `BLOG_ANALYTICS_SITE_HOST`
- `BLOG_ANALYTICS_IP_SALT`
- `BLOG_ANALYTICS_COLLECT_TOKEN_SECRET`

Analytics 详细配置见 [站点分析配置](analytics-configuration.md)。

## 生产部署流程

GitHub Actions（`.github/workflows/deploy.yml`）负责测试、构建镜像、推送到腾讯云镜像仓库，并通过 SSH 远程执行 `docker compose`。服务器只负责拉取运行，MySQL / Redis 由宝塔管理，容器内只跑 `blog-server`。

| 分支 | 环境 | 容器名 | 配置覆盖 |
|------|------|------|------|
| `dev` | staging | `blog-server-test` | `config.test.yaml` |
| `main` | production | `blog-server` | `config.prod.yaml` |

服务器部署目录（`DEPLOY_DIR`，例如 `/root/docker/blog-backend`）为扁平结构：

```text
config/             # config.yaml + config.{prod,test}.yaml，CI 自动同步
geoip/              # ip2region_v4.xdb / ip2region_v6.xdb，按需手动放置
docker-compose.yml  # CI 自动同步
.env                # 密钥，首次手动创建
```

首次部署只需在服务器创建 `.env`：

```bash
cp .env.example .env
```

之后推送到 `dev` / `main` 会自动部署对应环境，并按 commit SHA 拉取精确镜像。

## 手动起停

进入 `DEPLOY_DIR`：

```bash
docker compose up -d
```

同目录 `.env` 会自动加载，镜像默认回退到 `:latest`。

## 数据库迁移

镜像内包含独立的 `blog-migrate`，API 服务启动本身不会修改表结构。生产库与 staging 在迁移工具接管前均已确认执行到 `20260830_user_like_lookup_index.sql`，GitHub Actions 会自动执行以下首次登记：

```bash
docker compose pull blog-server
docker compose run --rm --no-deps blog-server \
  ./blog-migrate adopt --through 20260830_user_like_lookup_index.sql
```

`adopt` 只登记历史，不执行 SQL。`20260830_user_like_lookup_index.sql` 是迁移工具接管旧数据库的固定历史基线，未来新增迁移时不得提高。

之后每次部署由 CI 在启动新镜像前自动执行 `up`。以下命令只用于手动部署或排障：

```bash
docker compose run --rm --no-deps blog-server ./blog-migrate status
docker compose run --rm --no-deps blog-server ./blog-migrate up
docker compose up -d --remove-orphans blog-server
```

CI 的实际顺序为：拉取精确 SHA 镜像、幂等登记固定历史基线、执行所有 pending 迁移、更新服务、轮询 `/health`，最后输出迁移状态。迁移出现 dirty 时会在更新服务前停止；应先按备份和实际表结构处理，不能直接重跑。

## 部署前检查

- `make test` 通过。
- `make swag` 已更新 Swagger 产物。
- 新增配置已同步到 `config/config.yaml`、环境覆盖文件和部署环境变量。
- 新增表结构已提供 `migrations/` SQL。
- 已在启动新镜像前执行 `blog-migrate up`，且不存在 pending / dirty 迁移。
- 启用内容审核前已按 [内容审核上线与回滚](moderation-rollout.md) 完成迁移与校验。
