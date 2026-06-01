# 留言板接口设计

## 目标

为留言板提供独立 HTTP 接口：分页获取留言、发表留言、点赞/取消点赞留言、删除留言。当前网站只允许给博主留言，因此 `owner_user_id` 默认值为 `1`；字段和查询参数仍保留，为未来支持给注册用户留言预留扩展点。

## 接口

- `GET /guestbook?owner_user_id=1&page=1&page_size=10`：公开分页查询留言。`owner_user_id` 可省略，默认 `1`；带合法 token 时返回当前用户是否已点赞。
- `POST /guestbook`：登录用户发表留言。请求体包含可选 `owner_user_id` 和必填 `content`。
- `POST /guestbook/:id/like`：登录用户切换点赞状态，未点赞则点赞，已点赞则取消。
- `DELETE /guestbook/:id`：登录用户删除留言。留言作者、留言板主人或管理员可删除。

## 分层

- `internal/handler/guestbook` 只绑定参数、读取 claims、调用 service、选择统一响应。
- `internal/service/guestbook` 负责默认 owner、内容清洗、权限角色判断和 DTO 转换。
- `internal/repository/guestbook` 负责 GORM 查询与事务，返回 model 或 repository 聚合结构。
- `internal/dto/guestbook.go` 是请求和响应结构唯一来源。

## 数据

留言记录继续使用现有 `model.Guestbook` 表。点赞复用 `user_like` 表，新增类型含义 `5=留言板留言`，通过唯一键保证同一用户对同一留言只有一条点赞记录，切换点赞时沿用软删除/恢复策略。

## 错误与测试

参数错误使用 `response.Fail(..., CodeBadRequest, ...)`；未登录、无权限、留言不存在分别返回统一的 401、403、404。测试覆盖 service 默认 owner 与内容清洗、handler claims 绑定、repository 点赞切换和删除权限。
