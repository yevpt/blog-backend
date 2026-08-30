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

镜像内包含独立的 `blog-migrate`，API 服务启动本身不会修改表结构。首次把现有生产库接入迁移台账时，先拉取新镜像，再查看状态并登记最后一个确认已经执行的迁移：

```bash
docker compose pull blog-server
docker compose run --rm --no-deps blog-server ./blog-migrate status
docker compose run --rm --no-deps blog-server \
  ./blog-migrate adopt --through <最后一个确认已执行的迁移.sql>
```

`adopt` 只登记历史，不执行 SQL。若当前所有迁移都已人工执行，可把 `--through` 指向仓库中最后一个迁移；否则只能指向实际完成的位置。

之后每次部署在启动新镜像前显式执行：

```bash
docker compose run --rm --no-deps blog-server ./blog-migrate status
docker compose run --rm --no-deps blog-server ./blog-migrate up
docker compose up -d --remove-orphans blog-server
```

当前 CI 不自动执行迁移，以免尚未完成首次 `adopt` 的生产库被误操作。迁移出现 dirty 时停止部署，先按备份和实际表结构处理，不能直接重跑。

## 部署前检查

- `make test` 通过。
- `make swag` 已更新 Swagger 产物。
- 新增配置已同步到 `config/config.yaml`、环境覆盖文件和部署环境变量。
- 新增表结构已提供 `migrations/` SQL。
- 已在启动新镜像前执行 `blog-migrate up`，且不存在 pending / dirty 迁移。
- 启用内容审核前已按 [内容审核上线与回滚](moderation-rollout.md) 完成迁移与校验。
