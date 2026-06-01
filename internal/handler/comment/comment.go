package comment

import commentservice "github.com/vpt/blog-backend/internal/service/comment"

// CommentHandler 评论模块 HTTP 入口，只负责参数绑定、调用 service 和选择响应。
type CommentHandler struct {
	svc commentservice.CommentService
}

// NewCommentHandler 创建评论 HTTP 处理器。
func NewCommentHandler(svc commentservice.CommentService) *CommentHandler {
	return &CommentHandler{svc: svc}
}
