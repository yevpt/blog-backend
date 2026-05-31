# pkg/storage

`pkg/storage` 封装 Garage（S3 兼容对象存储）和 CDN 私有读 URL 生成能力。

阅读代码时先看 `storage.go`：这个文件是对外入口，只放外部调用者需要关心的类型、构造函数和公开方法。具体实现再按需要查看：

- `garage.go`：Garage/S3 客户端初始化、对象 URL 策略、S3 预签名。
- `cdn.go`：CDN TypeD 签名算法。
- `path.go`：bucket/object 路径清理和默认值处理。

## 配置

配置来自 `pkg/config` 中的 `GarageConfig` 和 `CDNConfig`：

```yaml
garage:
  endpoint: "https://garage-s3-api.example.com"
  region: "garage"
  bucket: "blog"
  accessKeyID: "your_access_key_id"
  secretAccessKey: "your_secret_access_key"
  cdn: true

cdn:
  host: "https://blog-oss.example.com"
  secret: "your_cdn_secret"
  signQueryName: "a"
  timestampQueryName: "b"
```

`garage.cdn` 控制 URL 生成策略：

- `true`：`ObjectURL` 返回 CDN 私有签名 URL，路径格式为 `/bucket/object`。
- `false`：`ObjectURL` 返回 Garage S3 `GetObject` 预签名 URL。

## 调用方式

创建客户端：

```go
store, err := storage.NewGarage(&cfg.Garage, &cfg.CDN)
if err != nil {
	return err
}
```

获取对象访问 URL：

```go
url, err := store.ObjectURL(ctx, "images/cat.jpg")
if err != nil {
	return err
}
```

只生成 S3 预签名 URL：

```go
url, err := store.PresignedObjectURL(ctx, "images/cat.jpg")
if err != nil {
	return err
}
```

需要直接操作 S3 时，可以取底层客户端和默认 bucket：

```go
s3Client := store.S3()
bucket := store.Bucket()
```

## 返回值约定

- 空对象名返回空字符串和 `nil` 错误。
- CDN 配置开启但 CDN 签名器未初始化时返回错误。
- S3 预签名失败时返回带业务上下文的错误。
- 对象名允许带前导 `/`，内部会统一清理。

## 测试

相关测试在 `garage_test.go` 中，覆盖 CDN 签名、空对象名、CDN URL、S3 预签名 URL 和错误返回。

运行：

```bash
go test ./pkg/storage
```
