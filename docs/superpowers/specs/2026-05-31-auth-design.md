# 认证模块设计文档

**日期**：2026-05-31  
**范围**：注册、登录、JWT 鉴权、限流防护

---

## 一、API 接口

| 方法 | 路径 | 是否需要登录 | 说明 |
|------|------|------------|------|
| `POST` | `/auth/send-code` | 否 | 发送邮箱验证码 |
| `POST` | `/auth/register` | 否 | 邮箱注册 |
| `POST` | `/auth/login` | 否 | 三合一登录 |
| `POST` | `/auth/refresh` | 否 | 刷新 token |

### 请求/响应结构

```
POST /auth/send-code
  Body:     { email }
  Response: {}
  说明：不暴露邮箱是否已注册，防止枚举

POST /auth/register
  Body:     { email, password, code, nickname? }
  Response: { user: { id, email, nickname } }
  说明：nickname 选填，未传则自动生成（邮箱前缀 + 4 位随机字符，保证唯一）

POST /auth/login
  Body:     { identifier, password }
  Response: { access_token, refresh_token, expires_in, user: { id, username, nickname, roles } }
  说明：identifier 可以是 username / email / phone，后端自动识别
        expires_in 单位为秒（access token 过期时间，固定 7200）

POST /auth/refresh
  Body:     { refresh_token }
  Response: { access_token, refresh_token, expires_in }
  说明：token rotation，旧 refresh token 作废，同时签发新的
        expires_in 单位为秒（7200）
```

---

## 二、分层架构

### 新增/修改文件

```
pkg/jwt/jwt.go                ← 修改：Claims 加 TokenType，新增 GenerateAccess / GenerateRefresh
pkg/email/email.go            ← 新建：封装阿里云 163 SMTP 发送（gomail.v2）
internal/dto/auth.go          ← 新建：RegisterReq、LoginReq、RefreshReq、LoginResp、TokenResp、UserResp
internal/repository/user.go   ← 填充：UserRepository 接口及实现
internal/service/auth.go      ← 新建：AuthService 接口及实现
internal/handler/auth.go      ← 新建：HTTP 层，参数绑定 + 调 service + 返回响应
internal/middleware/ratelimit.go ← 新建：Redis 双阈值限流 + IP 封禁
internal/router/router.go     ← 修改：注册 /auth/* 路由，挂载限流中间件
config/config.yaml            ← 修改：新增 email 配置节（不含密钥）
config/config.local.yaml      ← 修改：填入 SMTP 授权码（不提交 git）
```

> `internal/handler/user.go` 和 `internal/service/user.go` 保持空文件，认证逻辑独立放 auth.go。

---

## 三、核心数据流

### SendCode

1. 校验 email 格式
2. 检查邮箱维度频率限制（Redis）：
   - 同一 email 60 秒内只能发 1 次
   - 同一 email 10 分钟内最多 2 次
   - 同一 email 24 小时内最多 7 次
3. 生成 6 位数字验证码
4. 存 Redis，key=`email:code:{email}`，TTL=5 分钟
5. 通过 SMTP 发送验证码邮件

### Register

1. 校验参数（email 格式、密码长度 ≥ 8 位、验证码格式）
2. 从 Redis 取验证码，比对后立即删除（一次性）
3. 检查 email 唯一性
4. nickname 处理：
   - 未传 → 取邮箱 `@` 前缀最多 6 个字符 + 4 位随机字母数字，循环检测唯一性（最多重试 10 次，全部冲突则返回 500）
   - 已传 → 直接使用
5. username 字段：User model 要求非空唯一，email 注册时自动将 `username` 设置为 email 值，后续可通过修改个人资料更改
6. bcrypt hash 密码（cost=12）
7. 数据库事务：INSERT user + INSERT user_role（role_id=3，ROLE_NORMAL）
7. 返回用户基本信息

### Login

1. FindByIdentifier（SQL：`username=? OR email=? OR phone=?`）
2. 用户不存在时也执行 bcrypt 比对（防时序攻击），统一返回 401
3. bcrypt 比对密码
4. 检查 user.status == 1，否则返回 403
5. 查询用户角色列表
6. 生成 access token（2h）+ refresh token（7d）
7. 用 goroutine 异步更新 last_login_at（fire-and-forget，错误只记日志，不影响响应）
8. 返回 tokens + 用户信息

### Refresh

1. 解析 refresh token
2. 校验 `claims.TokenType == "refresh"`，否则返回 401
3. 生成新 access token（2h）+ 新 refresh token（7d）（token rotation）
4. 返回新 tokens

---

## 四、JWT 扩展

```go
type Claims struct {
    UserId    int64    `json:"uid"`
    Username  string   `json:"username"`
    Roles     []string `json:"roles"`
    TokenType string   `json:"type"` // "access" | "refresh"
    jwtlib.RegisteredClaims
}

// 新增方法
func (m *Manager) GenerateAccess(userId int64, username string, roles []string) (string, error)
func (m *Manager) GenerateRefresh(userId int64, username string, roles []string) (string, error)
```

**Auth 中间件**额外校验 `claims.TokenType == "access"`，防止 refresh token 被误用为 access token。

---

## 五、Repository 接口

```go
type UserRepository interface {
    FindByIdentifier(identifier string) (*model.User, error)
    FindByID(id uint) (*model.User, error)
    ExistsByEmail(email string) (bool, error)
    ExistsByNickname(nickname string) (bool, error)
    Create(user *model.User, roleID uint) error          // 事务：插 user + user_role
    FindRolesByUserID(userID uint) ([]string, error)
    UpdateLastLoginAt(userID uint) error
}
```

---

## 六、Service 接口

```go
type AuthService interface {
    SendCode(email string, ip string) error
    Register(req *dto.RegisterReq) (*dto.UserResp, error)
    Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error)
    Refresh(refreshToken string) (*dto.TokenResp, error)
}
```

---

## 七、限流中间件

### IP 维度（`middleware.RateLimitStrict` / `middleware.RateLimitNormal`）

```
Strict：同一 IP，60s 内 >5 次 → 429；>20 次 → 封禁 15 分钟
Normal：同一 IP，60s 内 >10 次 → 429；>30 次 → 封禁 15 分钟
```

Redis key：
- `ratelimit:{route_path}:{ip}` → 滑动窗口计数，TTL=Window
- `ban:ip:{ip}` → 封禁标记，TTL=BanDuration（**全局**，触发后影响所有请求）

路由挂载方式（`c.FullPath()` 自动派生 key，无需手动传名称）：

```go
r.POST("/auth/send-code", middleware.RateLimitStrict(redisClient), authHandler.SendCode)
r.POST("/auth/register",  middleware.RateLimitStrict(redisClient), authHandler.Register)
r.POST("/auth/login",     middleware.RateLimitNormal(redisClient), authHandler.Login)
```

### 邮箱维度（service 层校验）

| 窗口 | 限制 | Redis Key |
|------|------|-----------|
| 60 秒 | 1 次（最小冷却） | `email:cd:{email}` TTL=60s |
| 10 分钟 | 2 次 | `email:10m:{email}` TTL=10min |
| 24 小时 | 7 次 | `email:1d:{email}` TTL=24h |

---

## 八、错误处理

| 场景 | HTTP 状态 | 说明 |
|------|-----------|------|
| 参数格式错误 | 400 | email 格式、密码长度等 |
| 验证码错误/过期 | 400 | 统一提示"验证码无效" |
| email 已注册 | 400 | "该邮箱已被注册" |
| 用户不存在 / 密码错误 | 401 | **统一提示**，不区分，防枚举 |
| 用户被禁用 | 403 | "账号已被禁用" |
| refresh token 无效/过期 | 401 | 让前端重新登录 |
| IP 触发软限制 | 429 | 携带 `Retry-After` header |
| IP 触发封禁 | 429 | 响应体含封禁剩余时间 |
| 邮箱发送频率超限 | 429 | "发送过于频繁，请稍后再试" |

---

## 九、邮件配置

SMTP 使用 163 邮箱，配置拆分存放：

```yaml
# config.yaml（提交 git）
email:
  host: smtp.163.com
  port: 465
  from: vpt940417@163.com

# config.local.yaml（不提交 git）
email:
  password: <SMTP授权码>
```

邮件发送使用 `gopkg.in/gomail.v2`，封装在 `pkg/email/email.go`，通过构造函数注入。

---

## 十、扩展点（本次不实现）

- **第三方登录（OAuth）**：`model/social_user.go` 已预留 SocialUser / SocialUserAuth 表结构，后续实现 GitHub / Gitee 等登录时接入
- **GoCaptcha 行为验证**：在限流第一阈值触发时替代 429，引导用户完成滑块验证；需前后端联动，独立迭代
