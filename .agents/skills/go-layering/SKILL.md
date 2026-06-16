---
name: "go-layering"
description: "Use when writing or modifying code in internal/handler, internal/service, or internal/repository — defines layer boundaries, dependency injection, interface-for-mock rules, package organization, and the interface{}/any policy. Trigger whenever adding a service method, a repository query, wiring dependencies, or creating a new file/package under these layers."
license: "MIT"
metadata:
  scope: "project"
---

# 分层架构

```text
handler/    绑参数、调 service、选 response。禁 SQL/GORM/业务规则。
service/    业务逻辑。只依赖 repository 接口。
repository/ 数据查询。返回 model.*，禁返回 dto.*。
dto/        对外请求/响应的唯一来源。
model/      GORM 表结构，禁直接给前端或写进 Swagger。
```

## 边界

- handler 不写 SQL/GORM/业务判断；返回前把 `model.*` 转成 `dto.*`。
- service 不碰 GORM，只调 repository 接口。
- repository 不返回 `dto.*`，不含业务策略。

## 依赖注入

- db、redis、logger、mailer、jwt manager 等全部构造注入；**禁全局变量**。
- service 与 repository 都定义接口，供 gomock 测试。

## 包组织（避免现存的割裂）

- 一个层内风格统一：要么扁平文件，要么子目录拆分，**禁两者并存**。
- **禁同名扁平文件 + 子目录共存**（如 `service/comment.go` 与 `service/comment/`）——新代码无法判断归属。
- 每个包要有门面入口文件（`service.go`/`user.go`），只放对外构造函数、接口、公开方法；实现细节拆到同包其他文件。

## interface{} 口径

- 接口、DTO、业务方法签名：**禁 `interface{}`**，用泛型或具体类型。
- 仅数据层动态字段（GORM partial update）允许 `map[string]any`。
- 字面量统一写 `any`，不写 `interface{}`。
