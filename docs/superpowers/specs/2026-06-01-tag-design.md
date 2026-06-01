# 标签模块设计文档

**日期**：2026-06-01  
**范围**：标签公开查询、后台管理、标签文章关联维护

---

## 一、设计原则

标签模块沿用当前后端分层：handler 只处理 HTTP 绑定和统一响应，service 承载校验和业务语义，repository 封装 GORM 查询和事务，DTO 是唯一对外结构来源。

标签下文章分页不重新实现文章聚合逻辑，而是复用文章服务已有的公开文章分页能力，并通过 `tag_id` 过滤保持响应、排序、封面 URL 解析和公开状态过滤一致。

## 二、HTTP 接口

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/tags` | 获取全部标签及公开文章数量 |
| `GET` | `/tags/:id` | 获取单个标签信息 |
| `GET` | `/tags/:id/articles` | 分页获取标签下公开文章 |

### 管理员接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/admin/tags` | 新增标签 |
| `PUT` | `/admin/tags/:id` | 编辑标签 |
| `DELETE` | `/admin/tags/:id` | 删除标签并清空文章标签关联 |
| `POST` | `/admin/tags/:id/articles` | 给单篇或多篇文章添加标签 |
| `DELETE` | `/admin/tags/:id/articles` | 从单篇或多篇文章移除标签 |

## 三、请求与响应

标签字段对齐 `model.Tag`：`name`、`url`、`icon`、`description`、`cover_img_url`、`seq`。

新增标签要求 `name` 和 `seq`；其他字段可选。编辑标签时未传字段保持原值，可选字符串传空字符串表示清空。文章关联维护请求统一使用：

```json
{
  "article_ids": [1, 2]
}
```

`article_ids` 支持单个和批量。service 会过滤 0、保持首次出现顺序并去重；归一化后为空时返回业务参数错误。

标签响应包含公开文章数量：

```json
{
  "id": 1,
  "name": "Go",
  "url": "go",
  "icon": "icons/go.svg",
  "description": "Go 语言相关内容",
  "cover_img_url": "covers/go.jpg",
  "seq": 0,
  "article_count": 12
}
```

`GET /tags/:id/articles` 使用 `page`、`page_size` 查询参数，响应复用 `dto.ArticlePageResp`。

## 四、数据处理

删除标签时软删除 `tag`，并硬删除 `article_tag` 中该标签的关联；文章本身不删除。

添加文章标签时先校验标签存在和文章存在，再批量插入 `article_tag`。重复关系不应让请求失败，使用 MySQL `ON DUPLICATE KEY DO NOTHING` 语义跳过已有关系。

移除文章标签时只删除当前标签与请求文章的关联，不校验文章是否仍存在，返回实际影响的关联数量。

## 五、错误处理

| 场景 | HTTP 状态 | 说明 |
|------|-----------|------|
| 参数错误 | 200 | `response.Fail(CodeBadRequest, "...")` |
| 未登录 | 401 | 管理员接口需要 token |
| 权限不足 | 403 | 管理员接口角色不足 |
| 标签或文章不存在 | 404 | ID 不存在或已软删除 |
| 服务器错误 | 500 | DB 或其他非预期错误 |

## 六、测试策略

- Repository：使用 `go-sqlmock` 覆盖列表计数、查单个、删除清关联、批量添加、批量移除。
- Service：使用 gomock mock repository 覆盖字段清洗、ID 去重、错误映射和 DTO 转换。
- Handler：使用 `httptest` 覆盖绑定、统一响应和错误映射。
- Swagger：新增接口后执行 `make swag`，检查生成文档包含标签路径。
- 最终验证：先跑标签相关包测试，再跑 `go test ./...`。
