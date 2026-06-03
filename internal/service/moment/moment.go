package moment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/dto"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	// ErrMomentInvalid 表示碎语参数不合法。
	ErrMomentInvalid = errors.New("碎语参数错误")
	// ErrMomentNotFound 表示碎语不存在。
	ErrMomentNotFound = errors.New("碎语不存在")
	// ErrMomentAuthorNotFound 表示碎语作者不存在。
	ErrMomentAuthorNotFound = errors.New("碎语作者不存在")
	// ErrMomentContentRequired 表示碎语正文不能为空。
	ErrMomentContentRequired = errors.New("碎语内容不能为空")
	// ErrMomentNoPermission 表示当前用户无权操作该碎语。
	ErrMomentNoPermission = errors.New("无权操作碎语")
	// ErrMomentTopLimitExceeded 表示置顶碎语数量已达上限。
	ErrMomentTopLimitExceeded = errors.New("最多置顶三条碎语")
)

// MomentService 碎语业务接口，负责查询、发布、删除、置顶、点赞和阅读计数。
type MomentService interface {
	List(req dto.MomentListReq, viewerID *uint) (*dto.MomentPageResp, error)
	GetDetail(id uint, viewerID *uint) (*dto.MomentItemResp, error)
	Save(req dto.MomentSaveReq, operatorID uint, roleNames []string) (*dto.MomentItemResp, error)
	Delete(id uint, operatorID uint, roleNames []string) (*dto.MomentDeleteResp, error)
	SetTop(id uint, operatorID uint, roleNames []string) (*dto.MomentTopResp, error)
	RemoveTop(id uint, operatorID uint, roleNames []string) (*dto.MomentTopResp, error)
	Read(id uint) (*dto.MomentReadResp, error)
	IsLiked(id uint, userID uint) (*dto.MomentLikeResp, error)
	ToggleLike(id uint, userID uint) (*dto.MomentItemResp, error)
}

type momentService struct {
	repo              momentrepo.MomentRepository
	objectURLResolver storage.ObjectURLResolver
}

// NewMomentService 创建碎语业务服务实例。
func NewMomentService(repo momentrepo.MomentRepository, objectURLResolver storage.ObjectURLResolver) MomentService {
	return &momentService{repo: repo, objectURLResolver: objectURLResolver}
}
