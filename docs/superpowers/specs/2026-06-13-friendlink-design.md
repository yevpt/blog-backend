# 友情链接接口设计

## 背景

当前项目已有 `friend_link` 表模型和空的 handler/service/repository 文件。本次补齐友情链接接口，不兼容旧 Java 项目的 `/link/*` 路径，统一使用当前项目的 REST 风格路径。

旧 Java 项目字段和行为仅作参考：公开列表、公开详情、管理员保存、管理员删除，以及返回前将头像对象路径转换为可访问 URL。

## 接口范围

公开接口：

- `GET /friend-links`：分页查询显示中的友情链接。
- `GET /friend-links/:id`：查询显示中的友情链接详情。

管理接口：

- `GET /admin/friend-links`：分页查询友情链接，支持按 `status` 过滤。
- `POST /admin/friend-links`：创建友情链接。
- `PUT /admin/friend-links/:id`：更新友情链接。
- `DELETE /admin/friend-links/:id`：删除友情链接。

## 字段设计

响应字段：

- `id`：友情链接 ID。
- `name`：网站名称。
- `description`：网站描述。
- `email`：站长邮箱。
- `phone`：联系电话。
- `site`：网站 URL。
- `avatar_url`：网站头像或 Logo 地址。
- `seq`：排序值，越小越靠前。
- `status`：状态，`0` 隐藏，`1` 显示。
- `created_at` / `updated_at`：创建和更新时间。

创建请求：

- 必填：`name`、`site`、`seq`。
- 可选：`description`、`email`、`phone`、`avatar_url`、`status`。
- `status` 未传时默认显示。

更新请求：

- 所有字段可选，未传字段保持原值。
- 可选字符串传空字符串表示清空。
- `status` 只允许 `0` 或 `1`。

## 行为规则

- 公开列表和公开详情只返回 `status=1` 且未软删除的数据。
- 管理列表可查看全部未软删除数据，并可用 `status` 过滤。
- 删除使用 GORM 软删除，不用 `status=0` 代替删除。
- 列表排序为 `seq ASC, id DESC`。
- 分页参数沿用项目习惯：`page` 从 1 开始，`page_size` 默认 10，最大 50。
- `avatar_url` 存库保持原始值：可以是外链，也可以是对象存储 key。
- 返回 DTO 前调用 `storage.ResolvePtrURL`：外链原样返回，对象 key 转为 Garage/CDN 访问 URL，行为参考用户头像。

## 分层设计

- `internal/handler/friendlink.go`：只做参数绑定、调用 service、写统一响应和 Swagger 注解。
- `internal/service/friendlink.go`：校验参数、分页归一化、model 到 DTO 转换、`avatar_url` URL 解析。
- `internal/repository/friendlink.go`：封装 GORM 查询和变更，只返回 `model.FriendLink`。
- `internal/dto/friendlink.go`：定义请求和响应结构，禁止暴露 `model.*` 给前端。
- `internal/router/router.go`：注册公开和管理员路由。

## 错误处理

- 参数错误返回 `response.Fail(c, response.CodeBadRequest, "...")`，HTTP 状态仍为 200。
- 未登录、无权限由现有 middleware 返回 401/403。
- 公开详情和管理变更找不到目标时返回 404。
- 数据库或 URL 解析之外的未知错误返回 500。
- `avatar_url` 解析失败时保留原始值，不阻断主流程，保持与现有用户头像解析行为一致。

## 测试与验证

- Service 测试：
  - 公开列表只转换可见数据并解析 `avatar_url`。
  - 公开详情找不到或隐藏时返回不存在。
  - 创建时校验必填字段和默认 `status`。
  - 更新时校验可选字段、清空字段、非法 `status`。
- Handler 测试：
  - 参数绑定失败返回业务参数错误。
  - service 的 not found 映射到 404。
- Repository 测试：
  - 使用 `go-sqlmock` 覆盖列表排序、按状态过滤、软删除。
- 文档验证：
  - 修改接口后执行 `make swag`，确认 `docs/swagger.yaml/json` 出现 `friend-links` 路径。
- 最终验证：
  - 至少执行相关包测试。
  - 公共行为变更后执行 `go test ./...`。
