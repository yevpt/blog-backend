---
name: "go-testing"
description: "Use when writing or modifying tests in this Go backend — covers which tool to use per layer (go-sqlmock for repository, gomock for service, httptest+testify for handler), test package naming, and which tests to run after changes. Trigger whenever adding a _test.go file, changing test coverage, or modifying an interface that has tests."
license: "MIT"
metadata:
  scope: "project"
---

# 测试

按层选工具：

- **repository**：`go-sqlmock`。
- **service**：`gomock` mock repository，核心业务必须覆盖。
- **handler**：`httptest` + `testify`。

约定：

- 测试文件 `xxx_test.go`，包名用 `_test` 后缀；仅在确需访问内部实现时用同包测试。
- 改接口至少跑相关包测试；改公共逻辑跑 `go test ./...`。
