# 碎语接口设计

## 目标

用当前 Go 后端的分层与 REST 风格实现碎语（旧项目 say）后端接口，完整保留旧项目的列表、详情、发布更新、删除、置顶、点赞、阅读、图片、评论与通知能力。

## 接口

- `GET /moments`：公开分页列表，支持 `user_id`、`role_id`、`page`、`page_size`，可选登录态返回 `is_liked`。
- `GET /moments/{id}`：公开详情，返回作者、图片、点赞数、评论数、阅读数。
- `POST /moments/{id}/read`：公开阅读计数自增。
- `GET /moments/{id}/like`：登录用户查询点赞状态。
- `POST /moments/{id}/like`：登录用户切换点赞。
- `POST /moments`：登录用户新增或更新自己的碎语；管理员可代管。
- `DELETE /moments/{id}`：作者或管理员删除。
- `POST /moments/{id}/top`：作者或管理员置顶，每个用户最多 3 条。
- `DELETE /moments/{id}/top`：作者或管理员取消置顶。
- 评论继续复用 `POST /comments`、`GET /comments?target_type=moment&target_id=...`，补齐 moment 评论通知。

## 数据

- 主表使用 `model.Moment`，状态 `1=公开`、`0=隐藏`，删除使用 GORM 软删除。
- 图片使用 `model.Media`，`owner_type=2` 表示碎语，`type=0` 表示图片。
- 点赞使用 `model.UserLike`，`type=3` 表示碎语点赞。
- 评论使用 `model.MomentComment`，回复使用现有 `comment_reply`。
- 通知使用 `model.Message` 与 `model.UserMessage`。

## 分层

- `internal/handler/moment` 只负责参数绑定、JWT claims、调用 service、统一响应和 Swagger。
- `internal/service/moment` 负责参数归一、内容清理、权限判断、DTO 转换和对象 URL 解析。
- `internal/repository/moment` 负责 GORM 查询、图片同步、点赞切换、阅读计数、通知写入。
- `internal/dto/moment.go` 是请求与响应唯一来源。
- `internal/router/router.go` 注册公开、登录和管理路由。

## 测试

- Repository 使用 `go-sqlmock` 覆盖列表聚合、保存图片、点赞与置顶约束。
- Service 使用 fake repository 覆盖内容清理、分页默认值、权限与错误映射。
- Handler 使用 `httptest` 覆盖参数绑定、登录用户透传和错误响应。
- 修改 Swagger 后执行 `make swag`，再执行相关包测试与 `go test ./...`。
