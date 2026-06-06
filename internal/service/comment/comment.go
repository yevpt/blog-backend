package comment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/dto"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	// ErrCommentTargetInvalid 表示 target_type 或 target_id 不合法。
	ErrCommentTargetInvalid = errors.New("评论目标参数错误")
	// ErrCommentTargetNotFound 表示评论目标不存在。
	ErrCommentTargetNotFound = errors.New("评论目标不存在")
	// ErrCommentClosed 表示目标已关闭评论。
	ErrCommentClosed = errors.New("评论已关闭")
	// ErrCommentNotFound 表示一级评论不存在。
	ErrCommentNotFound = errors.New("评论不存在")
	// ErrCommentReplyNotFound 表示回复不存在。
	ErrCommentReplyNotFound = errors.New("回复不存在")
	// ErrCommentContentRequired 表示评论或回复内容不能为空。
	ErrCommentContentRequired = errors.New("评论内容不能为空")
	// ErrCommentNoDeletePermission 表示当前用户无权删除评论。
	ErrCommentNoDeletePermission = errors.New("无权删除评论")
)

// CommentService 评论业务接口，负责评论、回复的查询、创建和删除。
type CommentService interface {
	List(targetType string, targetID uint, req dto.CommentListReq, viewerID *uint) (*dto.CommentPageResp, error)
	Create(targetType string, targetID uint, req dto.CommentCreateReq, userID uint) (*dto.CommentItemResp, error)
	ListReplies(targetType string, commentID uint, req dto.CommentReplyListReq, viewerID *uint) (*dto.CommentReplyPageResp, error)
	Reply(targetType string, commentID uint, req dto.CommentReplyCreateReq, userID uint) (*dto.CommentReplyResp, error)
	ToggleLike(targetType string, commentID uint, userID uint) (*dto.CommentLikeResp, error)
	ToggleReplyLike(targetType string, replyID uint, userID uint) (*dto.CommentLikeResp, error)
	DeleteComment(targetType string, commentID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error)
	DeleteReply(targetType string, replyID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error)
}

type commentService struct {
	repo              commentrepo.CommentRepository
	objectURLResolver storage.ObjectURLResolver
}

// NewCommentService 创建评论业务服务实例。
func NewCommentService(repo commentrepo.CommentRepository, objectURLResolver storage.ObjectURLResolver) CommentService {
	return &commentService{repo: repo, objectURLResolver: objectURLResolver}
}
