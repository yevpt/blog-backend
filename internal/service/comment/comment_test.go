package comment_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	"github.com/vpt/blog-backend/pkg/roles"
)

type fakeCommentRepo struct {
	createTarget  commentrepo.Target
	createUserID  uint
	createContent string
	createResp    *commentrepo.CommentAggregate
	createErr     error
	replyData     commentrepo.ReplyData
	replyResp     *commentrepo.ReplyAggregate
	replyErr      error
	deleteForce   bool
	deleteErr     error
}

func (f *fakeCommentRepo) List(target commentrepo.Target, page int, pageSize int) (*commentrepo.PageResult, error) {
	return &commentrepo.PageResult{Page: page, PageSize: pageSize}, nil
}

func (f *fakeCommentRepo) Create(target commentrepo.Target, userID uint, content string) (*commentrepo.CommentAggregate, error) {
	f.createTarget = target
	f.createUserID = userID
	f.createContent = content
	return f.createResp, f.createErr
}

func (f *fakeCommentRepo) Reply(data commentrepo.ReplyData) (*commentrepo.ReplyAggregate, error) {
	f.replyData = data
	return f.replyResp, f.replyErr
}

func (f *fakeCommentRepo) DeleteComment(target commentrepo.Target, commentID uint, userID uint, force bool) (*commentrepo.CommentRecord, error) {
	f.deleteForce = force
	return &commentrepo.CommentRecord{ID: commentID, UserID: userID}, f.deleteErr
}

func (f *fakeCommentRepo) DeleteReply(replyID uint, userID uint, force bool) (*model.CommentReply, error) {
	f.deleteForce = force
	return &model.CommentReply{Base: model.Base{ID: replyID}, FromUserID: userID}, f.deleteErr
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

	resp, err := svc.Create(dto.CommentCreateReq{
		TargetType: "article",
		TargetID:   3,
		Content:    "  好文章  ",
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.createTarget.Type)
	assert.Equal(t, uint(3), repo.createTarget.ID)
	assert.Equal(t, uint(7), repo.createUserID)
	assert.Equal(t, "好文章", repo.createContent)
	assert.Equal(t, uint(9), resp.ID)
	assert.Equal(t, "好文章", resp.Content)
}

func TestCommentService_Create_RejectsBlankContent(t *testing.T) {
	svc := commentservice.NewCommentService(&fakeCommentRepo{}, nil)

	_, err := svc.Create(dto.CommentCreateReq{
		TargetType: "article",
		TargetID:   3,
		Content:    "  ",
	}, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentContentRequired)
}

func TestCommentService_Create_MapsClosedTarget(t *testing.T) {
	repo := &fakeCommentRepo{createErr: commentrepo.ErrTargetCommentClosed}
	svc := commentservice.NewCommentService(repo, nil)

	_, err := svc.Create(dto.CommentCreateReq{
		TargetType: "article",
		TargetID:   3,
		Content:    "好文章",
	}, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentClosed)
}

func TestCommentService_Reply_PassesParentReplyID(t *testing.T) {
	now := time.Now()
	repo := &fakeCommentRepo{
		replyResp: &commentrepo.ReplyAggregate{
			Reply: model.CommentReply{
				Base:          model.Base{ID: 12, CreatedAt: now, UpdatedAt: now},
				CommentType:   uint8(commentrepo.TargetArticle),
				CommentID:     9,
				FromUserID:    7,
				ToUserID:      8,
				ParentReplyID: 11,
				Content:       "收到",
			},
		},
	}
	svc := commentservice.NewCommentService(repo, nil)

	resp, err := svc.Reply(9, dto.CommentReplyCreateReq{
		TargetType:    "article",
		ParentReplyID: 11,
		Content:       " 收到 ",
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, uint(9), repo.replyData.CommentID)
	assert.Equal(t, uint(11), repo.replyData.ParentReplyID)
	assert.Equal(t, "收到", repo.replyData.Content)
	assert.Equal(t, uint(12), resp.ID)
}

func TestCommentService_DeleteComment_AllowsAdminForceDelete(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := commentservice.NewCommentService(repo, nil)

	_, err := svc.DeleteComment("article", 9, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.True(t, repo.deleteForce)
}

func TestCommentService_List_MapsRepositoryErrors(t *testing.T) {
	repo := &fakeCommentRepo{}
	repo.createErr = errors.New("unused")
	svc := commentservice.NewCommentService(repo, nil)

	resp, err := svc.List(dto.CommentListReq{TargetType: "article", TargetID: 3, Page: 0, PageSize: 99})

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 50, resp.PageSize)
}
