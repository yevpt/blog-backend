package comment_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	"github.com/vpt/blog-backend/pkg/roles"
)

type fakeCommentRepo struct {
	listTarget           commentrepo.Target
	listPage             int
	listPageSize         int
	listViewerID         *uint
	listResp             *commentrepo.PageResult
	listErr              error
	createTarget         commentrepo.Target
	createUserID         uint
	createContent        string
	createResp           *commentrepo.CommentAggregate
	createErr            error
	listRepliesTarget    commentrepo.Target
	listRepliesCommentID uint
	listRepliesPage      int
	listRepliesPageSize  int
	listRepliesViewerID  *uint
	listRepliesResp      *commentrepo.ReplyPageResult
	listRepliesErr       error
	replyData            commentrepo.ReplyData
	replyResp            *commentrepo.ReplyAggregate
	replyErr             error
	toggleLikeTarget     commentrepo.Target
	toggleLikeCommentID  uint
	toggleLikeUserID     uint
	toggleLikeResp       *commentrepo.LikeResult
	toggleLikeErr        error
	deleteCommentForce   bool
	deleteReplyTarget    commentrepo.Target
	deleteReplyID        uint
	deleteReplyForce     bool
	deleteErr            error
}

func (f *fakeCommentRepo) List(target commentrepo.Target, viewerID *uint, page int, pageSize int) (*commentrepo.PageResult, error) {
	f.listTarget = target
	f.listViewerID = viewerID
	f.listPage = page
	f.listPageSize = pageSize
	return f.listResp, f.listErr
}

func (f *fakeCommentRepo) Create(target commentrepo.Target, userID uint, content string) (*commentrepo.CommentAggregate, error) {
	f.createTarget = target
	f.createUserID = userID
	f.createContent = content
	return f.createResp, f.createErr
}

func (f *fakeCommentRepo) ListReplies(target commentrepo.Target, commentID uint, viewerID *uint, page int, pageSize int) (*commentrepo.ReplyPageResult, error) {
	f.listRepliesTarget = target
	f.listRepliesCommentID = commentID
	f.listRepliesViewerID = viewerID
	f.listRepliesPage = page
	f.listRepliesPageSize = pageSize
	return f.listRepliesResp, f.listRepliesErr
}

func (f *fakeCommentRepo) Reply(data commentrepo.ReplyData) (*commentrepo.ReplyAggregate, error) {
	f.replyData = data
	return f.replyResp, f.replyErr
}

func (f *fakeCommentRepo) ToggleLike(target commentrepo.Target, commentID uint, userID uint) (*commentrepo.LikeResult, error) {
	f.toggleLikeTarget = target
	f.toggleLikeCommentID = commentID
	f.toggleLikeUserID = userID
	return f.toggleLikeResp, f.toggleLikeErr
}

func (f *fakeCommentRepo) ToggleReplyLike(target commentrepo.Target, replyID uint, userID uint) (*commentrepo.LikeResult, error) {
	return &commentrepo.LikeResult{IsLiked: true, LikeCount: 1}, nil
}

func (f *fakeCommentRepo) DeleteComment(target commentrepo.Target, commentID uint, userID uint, force bool) (*commentrepo.CommentRecord, error) {
	f.deleteCommentForce = force
	return &commentrepo.CommentRecord{ID: commentID, UserID: userID}, f.deleteErr
}

func (f *fakeCommentRepo) DeleteReply(target commentrepo.Target, replyID uint, userID uint, force bool) (*commentrepo.ReplyRecord, error) {
	f.deleteReplyTarget = target
	f.deleteReplyID = replyID
	f.deleteReplyForce = force
	return &commentrepo.ReplyRecord{ID: replyID, FromUserID: userID}, f.deleteErr
}

func TestCommentService_List_UsesViewerAndPaging(t *testing.T) {
	viewerID := uint(9)
	repo := &fakeCommentRepo{
		listResp: &commentrepo.PageResult{Page: 2, PageSize: 50},
	}
	svc := commentservice.NewCommentService(repo, nil)

	resp, err := svc.List("article", 3, dto.CommentListReq{Page: 0, PageSize: 99}, &viewerID)

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.listTarget.Type)
	assert.Equal(t, uint(3), repo.listTarget.ID)
	assert.Equal(t, 1, repo.listPage)
	assert.Equal(t, 50, repo.listPageSize)
	require.NotNil(t, repo.listViewerID)
	assert.Equal(t, uint(9), *repo.listViewerID)
	assert.Equal(t, 2, resp.Page)
}

func TestCommentService_Create_TrimsContentAndMapsArticleTarget(t *testing.T) {
	now := time.Now()
	repo := &fakeCommentRepo{
		createResp: &commentrepo.CommentAggregate{
			Comment: commentrepo.CommentRecord{
				ID:        9,
				TargetID:  3,
				UserID:    7,
				Content:   "好文章",
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	svc := commentservice.NewCommentService(repo, nil)

	resp, err := svc.Create("article", 3, dto.CommentCreateReq{
		Content: "  好文章  ",
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.createTarget.Type)
	assert.Equal(t, uint(3), repo.createTarget.ID)
	assert.Equal(t, uint(7), repo.createUserID)
	assert.Equal(t, "好文章", repo.createContent)
	assert.Equal(t, uint(9), resp.ID)
	assert.Equal(t, "好文章", resp.Content)
}

func TestCommentService_ListReplies_UsesViewerAndPaging(t *testing.T) {
	now := time.Now()
	viewerID := uint(9)
	repo := &fakeCommentRepo{
		listRepliesResp: &commentrepo.ReplyPageResult{
			Page:     2,
			PageSize: 5,
			Replies: []commentrepo.ReplyAggregate{
				{
					Reply: commentrepo.ReplyRecord{
						ID:            12,
						CommentID:     9,
						FromUserID:    7,
						ToUserID:      8,
						ParentReplyID: 11,
						Content:       "收到",
						CreatedAt:     now,
						UpdatedAt:     now,
					},
				},
			},
		},
	}
	svc := commentservice.NewCommentService(repo, nil)

	resp, err := svc.ListReplies("article", 9, dto.CommentReplyListReq{Page: 2, PageSize: 5}, &viewerID)

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.listRepliesTarget.Type)
	assert.Equal(t, uint(9), repo.listRepliesCommentID)
	assert.Equal(t, 2, repo.listRepliesPage)
	assert.Equal(t, 5, repo.listRepliesPageSize)
	require.NotNil(t, repo.listRepliesViewerID)
	assert.Equal(t, uint(9), *repo.listRepliesViewerID)
	assert.Len(t, resp.List, 1)
	assert.Equal(t, uint(12), resp.List[0].ID)
}

func TestCommentService_Reply_PassesParentReplyID(t *testing.T) {
	now := time.Now()
	repo := &fakeCommentRepo{
		replyResp: &commentrepo.ReplyAggregate{
			Reply: commentrepo.ReplyRecord{
				ID:            12,
				CommentID:     9,
				FromUserID:    7,
				ToUserID:      8,
				ParentReplyID: 11,
				Content:       "收到",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		},
	}
	svc := commentservice.NewCommentService(repo, nil)

	resp, err := svc.Reply("article", 9, dto.CommentReplyCreateReq{
		ParentReplyID: 11,
		Content:       " 收到 ",
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.replyData.Target.Type)
	assert.Equal(t, uint(9), repo.replyData.CommentID)
	assert.Equal(t, uint(11), repo.replyData.ParentReplyID)
	assert.Equal(t, "收到", repo.replyData.Content)
	assert.Equal(t, uint(12), resp.ID)
}

func TestCommentService_ToggleLike_InvalidID(t *testing.T) {
	svc := commentservice.NewCommentService(&fakeCommentRepo{}, nil)

	_, err := svc.ToggleLike("article", 0, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentTargetInvalid)
}

func TestCommentService_ToggleLike_ReturnsLatestState(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleLikeResp: &commentrepo.LikeResult{IsLiked: true, LikeCount: 3},
	}
	svc := commentservice.NewCommentService(repo, nil)

	resp, err := svc.ToggleLike("article", 9, 7)

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.toggleLikeTarget.Type)
	assert.Equal(t, uint(9), repo.toggleLikeCommentID)
	assert.Equal(t, uint(7), repo.toggleLikeUserID)
	assert.True(t, resp.IsLiked)
	assert.Equal(t, int64(3), resp.LikeCount)
}

func TestCommentService_Create_RejectsBlankContent(t *testing.T) {
	svc := commentservice.NewCommentService(&fakeCommentRepo{}, nil)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{
		Content: "  ",
	}, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentContentRequired)
}

func TestCommentService_Create_MapsClosedTarget(t *testing.T) {
	repo := &fakeCommentRepo{createErr: commentrepo.ErrTargetCommentClosed}
	svc := commentservice.NewCommentService(repo, nil)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{
		Content: "好文章",
	}, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentClosed)
}

func TestCommentService_DeleteComment_AllowsAdminForceDelete(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := commentservice.NewCommentService(repo, nil)

	_, err := svc.DeleteComment("article", 9, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.True(t, repo.deleteCommentForce)
}

func TestCommentService_DeleteReply_UsesTargetPrefix(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := commentservice.NewCommentService(repo, nil)

	_, err := svc.DeleteReply("article", 12, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.deleteReplyTarget.Type)
	assert.Equal(t, uint(12), repo.deleteReplyID)
	assert.True(t, repo.deleteReplyForce)
}

func TestCommentService_List_MapsRepositoryErrors(t *testing.T) {
	repo := &fakeCommentRepo{listErr: errors.New("boom")}
	svc := commentservice.NewCommentService(repo, nil)

	_, err := svc.List("article", 3, dto.CommentListReq{Page: 1, PageSize: 10}, nil)

	require.EqualError(t, err, "boom")
}
