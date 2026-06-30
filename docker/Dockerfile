# ─── 基础：Go 编译环境 + WebP（chai2010/webp 依赖 CGO/libwebp）──────────────
FROM golang:1.25-alpine AS base

RUN apk add --no-cache gcc musl-dev libwebp-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# ─── 阶段：测试（CI 使用 --target test，与正式编译环境一致）────────────────────
FROM base AS test

COPY . .
RUN CGO_ENABLED=1 go test ./...

# ─── 阶段：编译 ───────────────────────────────────────────────────────────────
FROM base AS builder

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/blog-server ./cmd/server

# ─── 阶段：运行 ───────────────────────────────────────────────────────────────
FROM alpine:3.20

WORKDIR /app

# ca-certificates/tzdata：HTTPS 与时区；libwebp：CGO 动态链接的运行时库
RUN apk --no-cache add ca-certificates tzdata libwebp && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

COPY --from=builder /app/bin/blog-server .

EXPOSE 8080

CMD ["./blog-server"]
