# ─── 阶段一：编译 ───────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 先复制 go.mod/go.sum 并下载依赖，利用 Docker 层缓存
# 只有依赖变化时才重新下载，代码变化时直接跳过此层
COPY go.mod go.sum ./
RUN go mod download

# 复制全部源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/blog-server ./cmd/server

# ─── 阶段二：运行 ───────────────────────────────────────────────────────────────
FROM alpine:3.20

WORKDIR /app

# 安装 CA 证书（向 S3/Garage 等服务发起 HTTPS 请求需要）和时区数据
RUN apk --no-cache add ca-certificates tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 只复制编译产物，镜像体积最小
COPY --from=builder /app/bin/blog-server .

# 配置文件从外部挂载（生产环境通过 docker-compose volumes 挂载）
# 这样修改配置不需要重新构建镜像
EXPOSE 8080

CMD ["./blog-server"]
