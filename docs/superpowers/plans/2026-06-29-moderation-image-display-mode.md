# Moderation Image Display Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为碎语图片响应补齐稳定的 `display_mode` 字段，明确区分原图、模糊预览和 GIF 审核占位图。

**Architecture:** `dto.MomentMediaResp` 作为唯一外部响应增加字符串枚举字段；碎语 service 根据审核图片投影的 `Approved` 和 `IsGIF` 生成显示模式，未启用审核的普通图片统一返回 `original`。评论和留言正文中的图片继续由审核投影改写 URL，不改变正文结构。

**Tech Stack:** Go 1.25、Gin、testify、gomock、swaggo/swag。

## Global Constraints

- 不修改权限、输入、数据库结构、对象存储行为或图片清理策略。
- `display_mode` 只允许 `original`、`blurred`、`gif_placeholder`。
- 已通过审核的 GIF 返回 `original`；只有未通过审核的 GIF 返回 `gif_placeholder`。
- 禁止向 Swagger 暴露 `model.*`。

---

### Task 1: 图片显示模式响应与投影

**Files:**
- Modify: `internal/dto/moment.go`
- Modify: `internal/service/moment/mapper.go`
- Test: `internal/service/moment/moment_test.go`
- Generated: `docs/docs.go`
- Generated: `docs/swagger.json`
- Generated: `docs/swagger.yaml`

**Interfaces:**
- Consumes: `moderationrepo.ImageView{Approved bool, IsGIF bool}`。
- Produces: `dto.MomentMediaResp.DisplayMode string`，JSON 字段名为 `display_mode`。

- [x] **Step 1: 写入失败测试**

在普通碎语图片映射断言 `display_mode == "original"`；在审核图片投影测试中同时构造已通过图片、未通过静态图片和未通过 GIF，并分别断言 `original`、`blurred`、`gif_placeholder`。

- [x] **Step 2: 验证测试因字段缺失而失败**

Run: `go test ./internal/service/moment -run 'TestMomentService(List|.*Moderation)' -count=1`

Expected: FAIL，错误指向 `MomentMediaResp` 尚无 `DisplayMode`。

- [x] **Step 3: 写入最小实现**

在 `MomentMediaResp` 增加：

```go
DisplayMode string `json:"display_mode" enums:"original,blurred,gif_placeholder" example:"blurred"`
```

在 `mapper.go` 增加只负责枚举映射的辅助函数：

```go
func moderationImageDisplayMode(image moderationrepo.ImageView) string {
	if image.Approved {
		return "original"
	}
	if image.IsGIF {
		return "gif_placeholder"
	}
	return "blurred"
}
```

审核图片映射调用该函数，普通业务图片映射固定返回 `original`。

- [x] **Step 4: 验证相关测试通过**

Run: `go test ./internal/service/moment -count=1`

Expected: PASS。

- [x] **Step 5: 生成并检查 Swagger**

Run: `make swag`

Expected: Swagger 中 `dto.MomentMediaResp.display_mode` 存在，并包含三个允许值；生成结果不含 `model.*`。

- [x] **Step 6: 全量验证**

Run: `go test ./... -count=1 && go vet ./... && git diff --check`

Expected: 全部退出码为 0。
