package guestbook

import (
	"errors"

	"github.com/vpt/blog-backend/internal/dto"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	"github.com/vpt/blog-backend/pkg/storage"
)

const defaultOwnerUserID uint = 1

var (
	// ErrGuestbookInvalid 表示留言板参数不合法。
	ErrGuestbookInvalid = errors.New("留言参数错误")
	// ErrGuestbookOwnerNotFound 表示留言板主人不存在。
	ErrGuestbookOwnerNotFound = errors.New("留言板主人不存在")
	// ErrGuestbookNotFound 表示留言不存在。
	ErrGuestbookNotFound = errors.New("留言不存在")
	// ErrGuestbookContentRequired 表示留言内容不能为空。
	ErrGuestbookContentRequired = errors.New("留言内容不能为空")
	// ErrGuestbookNoDeletePermission 表示当前用户无权删除留言。
	ErrGuestbookNoDeletePermission = errors.New("无权删除留言")
)

// GuestbookService 留言板业务接口，负责留言查询、发表、点赞和删除。
type GuestbookService interface {
	List(req dto.GuestbookListReq, viewerID *uint) (*dto.GuestbookPageResp, error)
	Create(req dto.GuestbookCreateReq, fromUserID uint) (*dto.GuestbookItemResp, error)
	ToggleLike(id uint, userID uint) (*dto.GuestbookLikeResp, error)
	Delete(id uint, userID uint, roleNames []string) (*dto.GuestbookDeleteResp, error)
}

type guestbookService struct {
	repo              guestbookrepo.GuestbookRepository
	objectURLResolver storage.ObjectURLResolver
}

// NewGuestbookService 创建留言板业务服务实例。
func NewGuestbookService(repo guestbookrepo.GuestbookRepository, objectURLResolver storage.ObjectURLResolver) GuestbookService {
	return &guestbookService{repo: repo, objectURLResolver: objectURLResolver}
}
