package dto

// UploadedImageFile 表示 multipart 中已读取出的单张图片文件。
type UploadedImageFile struct {
	Name string
	Data []byte
}
