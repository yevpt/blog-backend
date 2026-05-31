# 文章模块设计文档

**日期**：2026-05-31  
**范围**：文章公开阅读、后台管理、点赞、阅读计数

---

## 一、设计原则

文章模块按当前 Go 后端的新结构设计，不兼容旧 Java 项目的路径、字段命名或前端习惯。旧 `PostController` 只作为业务意图参考：文章可保存、分页查询、查看详情、点赞、删除、增加阅读数。

实现以清晰阅读路径为第一约束：

- 入口文件保持简洁，只暴露构造函数、公开接口和主要调用方式。
- handler 只做参数绑定、获取登录信息、调用 service、返回统一 response。
- service 承载业务规则，只依赖 repository 接口。
- repository 封装 GORM 查询和事务，返回 `model.*` 或聚合模型，不返回 `dto.*`。
- dto 是 HTTP 入参和出参的唯一来源，Swagger 不暴露 `model.*`。

## 二、HTTP 接口

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/articles/ids` | 返回公开文章 ID 列表 |
| `GET` | `/articles` | 分页查询公开文章 |
| `GET` | `/articles/:id` | 查询文章详情 |
| `POST` | `/articles/:id/read` | 阅读数增加 1 |

### 登录接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/articles/:id/like` | 查询当前用户是否已点赞 |
| `POST` | `/articles/:id/like` | 切换当前用户点赞状态 |

### 管理员接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/admin/articles` | 新增或更新文章 |
| `DELETE` | `/admin/articles/:id` | 软删除文章 |

## 三、请求结构

### 分页查询

`GET /articles`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `page` | int | 否 | 页码，从 1 开始，默认 1 |
| `page_size` | int | 否 | 每页数量，默认 10，最大 50 |
| `recommend` | bool | 否 | 只查询推荐文章 |
| `category_id` | uint | 否 | 按分类过滤 |
| `tag_id` | uint | 否 | 按标签过滤 |

分页内部实现使用稳定排序：推荐列表按 `article_recommend.seq ASC, article.created_at DESC, article.id DESC`；普通列表按 `article.created_at DESC, article.id DESC`。查询总数和列表查询分开执行，列表只返回页面需要的数据。

### 新增或更新

`POST /admin/articles`

```json
{
  "id": 1,
  "title": "文章标题",
  "cover_img_url": "https://example.com/cover.jpg",
  "short_content": "摘要",
  "content": "Markdown 正文",
  "status": 1,
  "comment_status": 1,
  "password": "",
  "category_ids": [1],
  "tag_ids": [1, 2],
  "music_ids": [3],
  "recommend": true,
  "recommend_seq": 10
}
```

约束：

- `id` 为空或 0 时创建文章；大于 0 时更新文章。
- `title`、`content` 必填。
- `status`：`0=隐藏`，`1=公开`，`2=加密`。
- `comment_status`：`0=关闭`，`1=开启`。
- `category_ids` 至少 1 个；当前数据库支持多分类关联。
- `tag_ids`、`music_ids` 可为空。
- `status=2` 时 `password` 必填；否则保存为 `NULL`。
- `recommend=true` 时写入或更新 `article_recommend`；否则删除推荐关系。

## 四、响应结构

### 列表分页

```json
{
  "total": 100,
  "pages": 10,
  "page": 1,
  "page_size": 10,
  "list": [
    {
      "id": 1,
      "title": "文章标题",
      "cover_img_url": "https://example.com/cover.jpg",
      "short_content": "摘要",
      "user_id": 1,
      "status": 1,
      "comment_status": 1,
      "read_count": 20,
      "like_count": 3,
      "comment_count": 2,
      "is_recommended": true,
      "created_at": "2026-05-31T10:00:00+08:00",
      "updated_at": "2026-05-31T10:00:00+08:00"
    }
  ]
}
```

列表不返回正文，避免分页接口搬运大字段。

### 详情

详情返回文章正文和关联信息：

- 文章基础字段。
- `category_ids` 和 `categories`。
- `tag_ids` 和 `tags`。
- `music_ids` 和 `music`。
- `like_count`、`comment_count`、`is_liked`。
- `is_recommended`、`recommend_seq`。

加密文章的读取策略：

- 管理员可直接读取完整内容。
- 公开详情接口首次实现不开放密码校验，`status=2` 的文章只返回基础信息和摘要，不返回 `content`。
- 后续如前端需要，可独立新增 `POST /articles/:id/unlock`，避免把密码放到 GET 查询参数。

## 五、分层与文件组织

```text
internal/dto/article.go
  文章请求 DTO、响应 DTO、分页 DTO。

internal/handler/article.go
  ArticleHandler 门面：构造函数、HTTP 方法、Swagger 注解。

internal/service/article.go
  ArticleService 接口、业务错误、主要业务编排。

internal/service/article_mapper.go
  model/聚合结果到 dto 的转换。

internal/repository/article.go
  ArticleRepository 接口、入口构造函数和薄转发。

internal/repository/article_query.go
  文章列表、详情、计数、过滤查询。

internal/repository/article_mutation.go
  保存、软删除、阅读数、点赞切换、推荐和关联表事务。

internal/router/router.go
  注册文章公开、登录、管理员路由。
```

入口文件职责保持克制：`handler/article.go`、`service/article.go`、`repository/article.go` 都不堆放底层查询细节。

## 六、数据处理

### 保存文章

service 校验请求，生成 `model.Article`，repository 在事务中执行：

1. 创建或更新 `article`。
2. 删除并重建 `article_category`。
3. 删除并重建 `article_tag`。
4. 删除并重建 `article_music`。
5. 按 `recommend` 写入、更新或删除 `article_recommend`。
6. 返回重新聚合后的详情。

### 分页查询

repository 使用明确的 filter 对象表达查询条件：

- 只返回 `status=1` 的公开文章。
- `recommend=true` 时 inner join `article_recommend`。
- `category_id` 和 `tag_id` 通过关联表过滤。
- 总数查询使用去重后的 article id 计数，避免关联表造成重复计数。
- 列表查询只取当前页文章，再批量聚合点赞数、评论数和推荐状态，避免 N+1 查询。

### 详情查询

详情查询按文章 ID 获取基础记录，再批量读取分类、标签、音乐、推荐、点赞数、评论数。当前用户 ID 可为空；为空时 `is_liked=false`。

### 点赞切换

`user_like` 使用 `type=1` 表示文章点赞。

- 未点赞：创建或恢复点赞记录。
- 已点赞：软删除点赞记录。
- 返回文章详情中的点赞相关状态。
- 点赞他人文章时创建 `message` 和 `user_message`，消息类型使用 `article_like`。

### 阅读数

阅读数使用数据库原子更新：`read_count = read_count + 1`。更新后返回最新文章详情或基础阅读响应，避免先读再写造成并发丢失。

## 七、错误处理

| 场景 | HTTP 状态 | 说明 |
|------|-----------|------|
| 参数错误 | 200 | `response.Fail(CodeBadRequest, "...")` |
| 未登录 | 401 | 点赞和管理员接口需要 token |
| 权限不足 | 403 | 管理员接口角色不足 |
| 文章不存在 | 404 | ID 不存在或已软删除 |
| 服务器错误 | 500 | DB 或其他非预期错误 |

业务错误仍遵循当前项目约定：参数和可预期业务失败使用 HTTP 200 + 业务 code，真实鉴权、权限、不存在和服务器错误使用对应 HTTP 状态。

## 八、测试策略

- Repository：使用 `go-sqlmock` 覆盖分页过滤、详情聚合、事务保存、点赞切换、阅读数原子更新。
- Service：使用 gomock 或手写 mock 覆盖参数校验、加密文章返回策略、点赞消息触发条件。
- Handler：使用 `httptest` 覆盖绑定参数、统一响应、404/401/403 映射。
- Swagger：新增或修改接口后执行 `make swag`，检查 `docs/swagger.yaml/json` 包含文章路径。
- 最终验证：先跑文章相关包测试，再跑 `go test ./...`。

## 九、暂不实现

- 旧 `/post/*` 路径兼容。
- 旧前端字段兼容。
- GET 详情接口通过查询参数传密码。
- 草稿预览、版本历史、全文搜索。
- 基于游标的分页。当前分页使用页码模型，但内部保证稳定排序、最大页大小和去重计数；如果文章量明显增长，再独立引入 cursor pagination。
