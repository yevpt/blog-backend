package comment_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	commentservice "github.com/vpt/blog-backend/internal/service/comment"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	moderationmock "github.com/vpt/blog-backend/internal/service/moderation/mock"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
)

type fakeCommentRepo struct {
	listTarget            commentrepo.Target
	listPage              int
	listPageSize          int
	listViewerID          *uint
	listResp              *commentrepo.PageResult
	listErr               error
	listAdminTargetType   uint8
	listAdminSearch       string
	listAdminPage         int
	listAdminPageSize     int
	listAdminResp         *commentrepo.AdminPageResult
	listAdminErr          error
	createTarget          commentrepo.Target
	createUserID          uint
	createContent         string
	createResp            *commentrepo.CommentAggregate
	createErr             error
	listRepliesTarget     commentrepo.Target
	listRepliesCommentID  uint
	listRepliesPage       int
	listRepliesPageSize   int
	listRepliesViewerID   *uint
	listRepliesResp       *commentrepo.ReplyPageResult
	listRepliesErr        error
	replyData             commentrepo.ReplyData
	replyResp             *commentrepo.ReplyAggregate
	replyErr              error
	toggleLikeTarget      commentrepo.Target
	toggleLikeCommentID   uint
	toggleLikeUserID      uint
	toggleLikeResp        *commentrepo.LikeResult
	toggleLikeErr         error
	toggleReplyLikeTarget commentrepo.Target
	toggleReplyLikeID     uint
	toggleReplyLikeUserID uint
	toggleReplyLikeResp   *commentrepo.LikeResult
	toggleReplyLikeErr    error
	deleteCommentForce    bool
	deleteReplyTarget     commentrepo.Target
	deleteReplyID         uint
	deleteReplyForce      bool
	deleteErr             error
}

func (f *fakeCommentRepo) List(target commentrepo.Target, viewerID *uint, page int, pageSize int) (*commentrepo.PageResult, error) {
	f.listTarget = target
	f.listViewerID = viewerID
	f.listPage = page
	f.listPageSize = pageSize
	return f.listResp, f.listErr
}

func (f *fakeCommentRepo) ListAdmin(targetType uint8, search string, page int, pageSize int) (*commentrepo.AdminPageResult, error) {
	f.listAdminTargetType = targetType
	f.listAdminSearch = search
	f.listAdminPage = page
	f.listAdminPageSize = pageSize
	return f.listAdminResp, f.listAdminErr
}

func (f *fakeCommentRepo) Create(target commentrepo.Target, userID uint, content string) (*commentrepo.CommentAggregate, error) {
	f.createTarget = target
	f.createUserID = userID
	f.createContent = content
	if f.createResp == nil && f.createErr == nil {
		f.createResp = &commentrepo.CommentAggregate{
			Comment: commentrepo.CommentRecord{
				ID:       9,
				TargetID: target.ID,
				UserID:   userID,
				Content:  content,
			},
		}
	}
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
	if f.replyResp == nil && f.replyErr == nil {
		f.replyResp = &commentrepo.ReplyAggregate{
			Reply: commentrepo.ReplyRecord{
				ID:            12,
				CommentID:     data.CommentID,
				FromUserID:    data.FromUserID,
				ParentReplyID: data.ParentReplyID,
				Content:       data.Content,
			},
		}
	}
	return f.replyResp, f.replyErr
}

func (f *fakeCommentRepo) ToggleLike(target commentrepo.Target, commentID uint, userID uint) (*commentrepo.LikeResult, error) {
	f.toggleLikeTarget = target
	f.toggleLikeCommentID = commentID
	f.toggleLikeUserID = userID
	return f.toggleLikeResp, f.toggleLikeErr
}

func (f *fakeCommentRepo) ToggleReplyLike(target commentrepo.Target, replyID uint, userID uint) (*commentrepo.LikeResult, error) {
	f.toggleReplyLikeTarget = target
	f.toggleReplyLikeID = replyID
	f.toggleReplyLikeUserID = userID
	if f.toggleReplyLikeResp != nil || f.toggleReplyLikeErr != nil {
		return f.toggleReplyLikeResp, f.toggleReplyLikeErr
	}
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
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

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

func TestCommentServiceListProjectsModerationInOneBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	viewerID := uint(7)
	repo := &fakeCommentRepo{listResp: &commentrepo.PageResult{
		Page: 1, PageSize: 10, Comments: []commentrepo.CommentAggregate{{
			Comment: commentrepo.CommentRecord{ID: 9, TargetID: 3, UserID: 7, Content: "业务表正文"},
		}},
	}}
	ref := moderationservice.SubjectRef{Type: moderationservice.SubjectArticleComment, ID: 9, RootID: 3}
	risk := moderationrepo.RiskLow
	status := moderationrepo.ReviewPending
	pending := "待审正文"
	moderationSvc.EXPECT().LoadViews(gomock.Any(), []moderationservice.SubjectRef{ref}, moderationservice.Viewer{
		Role: moderationrepo.ViewerAuthor, UserID: 7,
	}).Return(map[moderationservice.SubjectKey]moderationservice.View{
		ref.Key(): {
			PublicState: moderationrepo.PublicVisible, DisplayVersion: moderationrepo.DisplayPending,
			VisibleContent: "安全待审正文", HasPendingRevision: true,
			PendingContent: &pending, PendingRiskLevel: &risk, PendingReviewStatus: &status,
			CanInteract: false,
		},
	}, nil)
	svc := commentservice.NewCommentService(repo, nil, nil, nil, moderationSvc)

	resp, err := svc.List("article", 3, dto.CommentListReq{}, &viewerID)

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "安全待审正文", resp.List[0].Content)
	assert.Equal(t, "pending", resp.List[0].Moderation.DisplayVersion)
	assert.Equal(t, &pending, resp.List[0].Moderation.PendingContent)
	assert.False(t, resp.List[0].Moderation.CanInteract)
}

func TestCommentServiceRoutesAllCommentSubjectsThroughModeration(t *testing.T) {
	tests := []struct {
		name        string
		targetType  string
		wantSubject moderationservice.SubjectType
	}{
		{"article", "article", moderationservice.SubjectArticleComment},
		{"moment", "moment", moderationservice.SubjectMomentComment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			moderationSvc := moderationmock.NewMockService(ctrl)
			repo := &fakeCommentRepo{}
			moderationSvc.EXPECT().Submit(gomock.Any(), moderationservice.SubmitCommand{
				ActorID: 7, Subject: moderationservice.SubjectRef{Type: tt.wantSubject, RootID: 3},
				Content: "正文", IdempotencyKey: "create-key",
			}).Return(moderationservice.SubmitResult{
				Subject: moderationservice.SubjectRef{Type: tt.wantSubject, ID: 9, RootID: 3},
				ItemID:  20, RevisionID: 30, RiskLevel: moderationservice.RiskLow,
				Action: moderationservice.ActionPostReview, PublicState: moderationservice.PublicVisible,
				ReviewStatus: moderationservice.ReviewPending, Content: "正文",
				Message: "发布成功，内容会被审核。", HasPendingRevision: true,
			}, nil)
			svc := commentservice.NewCommentService(repo, nil, nil, nil, moderationSvc)

			resp, err := svc.Create(tt.targetType, 3, dto.CommentCreateReq{
				Content: "正文", IdempotencyKey: "create-key",
			}, 7)

			require.NoError(t, err)
			assert.Equal(t, uint(9), resp.ID)
			require.NotNil(t, resp.Moderation.ReviewStatus)
			assert.Equal(t, "pending", *resp.Moderation.ReviewStatus)
			assert.Zero(t, repo.createUserID, "业务仓储不得重复创建可见行")
		})
	}
}

func TestCommentServiceRoutesAllReplySubjectsThroughModeration(t *testing.T) {
	tests := []struct {
		name        string
		targetType  string
		wantSubject moderationservice.SubjectType
	}{
		{"article", "article", moderationservice.SubjectArticleCommentReply},
		{"moment", "moment", moderationservice.SubjectMomentCommentReply},
		{"guestbook", "guestbook", moderationservice.SubjectGuestbookReply},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			moderationSvc := moderationmock.NewMockService(ctrl)
			repo := &fakeCommentRepo{}
			parentID := uint64(0)
			moderationSvc.EXPECT().Submit(gomock.Any(), moderationservice.SubmitCommand{
				ActorID: 7, Subject: moderationservice.SubjectRef{
					Type: tt.wantSubject, RootID: 9, ParentID: &parentID,
				},
				Content: "回复", IdempotencyKey: "reply-key",
			}).Return(moderationservice.SubmitResult{
				Subject:   moderationservice.SubjectRef{Type: tt.wantSubject, ID: 12, RootID: 9, ParentID: &parentID},
				RiskLevel: moderationservice.RiskMedium, Action: moderationservice.ActionPreReview,
				PublicState: moderationservice.PublicPlaceholder, ReviewStatus: moderationservice.ReviewPending,
				Message: "内容已提交，等待人工审核。", HasPendingRevision: true,
			}, nil)
			svc := commentservice.NewCommentService(repo, nil, nil, nil, moderationSvc)

			resp, err := svc.Reply(tt.targetType, 9, dto.CommentReplyCreateReq{
				Content: "回复", IdempotencyKey: "reply-key",
			}, 7)

			require.NoError(t, err)
			assert.Equal(t, uint(12), resp.ID)
			assert.Empty(t, resp.Content)
			assert.Equal(t, "placeholder", resp.Moderation.PublicState)
			assert.Zero(t, repo.replyData.FromUserID, "业务仓储不得重复创建可见行")
		})
	}
}

func TestCommentServiceRoutesEditsThroughModeration(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	moderationSvc.EXPECT().Edit(gomock.Any(), moderationservice.EditCommand{
		ActorID: 7, Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectArticleComment, ID: 9},
		Content: "新正文", IdempotencyKey: "edit-comment",
	}).Return(moderationservice.SubmitResult{
		Subject:   moderationservice.SubjectRef{Type: moderationservice.SubjectArticleComment, ID: 9, RootID: 3},
		RiskLevel: moderationservice.RiskLow, PublicState: moderationservice.PublicVisible,
		ReviewStatus: moderationservice.ReviewPending, Content: "新正文", HasPendingRevision: true,
	}, nil)
	moderationSvc.EXPECT().Edit(gomock.Any(), moderationservice.EditCommand{
		ActorID: 7, Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectGuestbookReply, ID: 12},
		Content: "新回复", IdempotencyKey: "edit-reply",
	}).Return(moderationservice.SubmitResult{
		Subject:   moderationservice.SubjectRef{Type: moderationservice.SubjectGuestbookReply, ID: 12, RootID: 9},
		RiskLevel: moderationservice.RiskMedium, PublicState: moderationservice.PublicPlaceholder,
		ReviewStatus: moderationservice.ReviewPending, HasPendingRevision: true,
	}, nil)
	svc := commentservice.NewCommentService(&fakeCommentRepo{}, nil, nil, nil, moderationSvc)

	comment, err := svc.EditComment("article", 9, dto.CommentCreateReq{Content: "新正文", IdempotencyKey: "edit-comment"}, 7, nil)
	require.NoError(t, err)
	assert.Equal(t, "新正文", comment.Content)
	reply, err := svc.EditReply("guestbook", 12, dto.CommentReplyCreateReq{Content: "新回复", IdempotencyKey: "edit-reply"}, 7, nil)
	require.NoError(t, err)
	assert.Empty(t, reply.Content)
}

func TestCommentServiceSignalsImagesToModeration(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	moderationSvc.EXPECT().Submit(gomock.Any(), moderationservice.SubmitCommand{
		ActorID: 7, Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectArticleComment, RootID: 3},
		Content: "![图](temp/a.jpg)", ImageKeys: []string{"embedded-image"}, IdempotencyKey: "image-key",
	}).Return(moderationservice.SubmitResult{}, moderationservice.ErrImageReviewUnavailable)
	svc := commentservice.NewCommentService(&fakeCommentRepo{}, nil, nil, nil, moderationSvc)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{Content: "![图](temp/a.jpg)", IdempotencyKey: "image-key"}, 7)
	assert.ErrorIs(t, err, moderationservice.ErrImageReviewUnavailable)
}

func TestCommentService_ListAdmin_NormalizesFiltersAndMapsItems(t *testing.T) {
	now := time.Now()
	repo := &fakeCommentRepo{
		listAdminResp: &commentrepo.AdminPageResult{
			Total:    1,
			Page:     1,
			PageSize: 50,
			Comments: []commentrepo.AdminCommentAggregate{
				{
					TargetType: commentrepo.TargetMoment,
					Comment: commentrepo.CommentRecord{
						ID:        9,
						TargetID:  3,
						UserID:    7,
						Content:   "测试评论",
						CreatedAt: now,
						UpdatedAt: now,
					},
					User:       &model.User{Base: model.Base{ID: 7}, Username: "vpt"},
					ReplyCount: 2,
					LikeCount:  4,
				},
			},
		},
	}
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

	resp, err := svc.ListAdmin(dto.AdminCommentListReq{
		Page:       0,
		PageSize:   99,
		TargetType: "moment",
		Search:     "  测试评论  ",
	})

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetMoment), repo.listAdminTargetType)
	assert.Equal(t, "测试评论", repo.listAdminSearch)
	assert.Equal(t, 1, repo.listAdminPage)
	assert.Equal(t, 50, repo.listAdminPageSize)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "moment", resp.List[0].TargetType)
	assert.Equal(t, int64(2), resp.List[0].ReplyCount)
	assert.Equal(t, int64(4), resp.List[0].LikeCount)
	assert.Equal(t, 1, resp.Pages)
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
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

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

func TestCommentService_Create_NormalizesTempCommentImagesBeforeCreate(t *testing.T) {
	store := newCommentAssetStore()
	store.keys["temp/comments/7/images/cat.jpg"] = true
	store.keyMap["https://cdn.example.com/blog/temp/comments/7/images/cat.jpg?a=1"] = "temp/comments/7/images/cat.jpg"
	repo := &fakeCommentRepo{}
	svc := commentservice.NewCommentService(repo, store, nil, nil, nil)

	resp, err := svc.Create("article", 3, dto.CommentCreateReq{
		Content: " 看图 ![cat](https://cdn.example.com/blog/temp/comments/7/images/cat.jpg?a=1) ",
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, "看图 ![cat](comments/article/3/images/cat.jpg)", repo.createContent)
	assert.Equal(t, []commentAssetCopy{{
		source: "temp/comments/7/images/cat.jpg",
		target: "comments/article/3/images/cat.jpg",
	}}, store.copies)
	require.NotNil(t, resp)
	assert.Equal(t, "看图 ![cat](https://cdn.example.com/blog/comments/article/3/images/cat.jpg)", resp.Content)
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
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

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
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

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

func TestCommentService_Reply_NormalizesTempCommentImagesBeforeCreate(t *testing.T) {
	store := newCommentAssetStore()
	store.keys["temp/comments/7/images/reply.jpg"] = true
	store.keyMap["https://cdn.example.com/blog/temp/comments/7/images/reply.jpg"] = "temp/comments/7/images/reply.jpg"
	repo := &fakeCommentRepo{}
	svc := commentservice.NewCommentService(repo, store, nil, nil, nil)

	resp, err := svc.Reply("article", 9, dto.CommentReplyCreateReq{
		Content: `<img src="https://cdn.example.com/blog/temp/comments/7/images/reply.jpg">`,
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, `<img src="comments/article/replies/9/images/reply.jpg">`, repo.replyData.Content)
	assert.Equal(t, []commentAssetCopy{{
		source: "temp/comments/7/images/reply.jpg",
		target: "comments/article/replies/9/images/reply.jpg",
	}}, store.copies)
	require.NotNil(t, resp)
	assert.Equal(t, `<img src="https://cdn.example.com/blog/comments/article/replies/9/images/reply.jpg">`, resp.Content)
}

func TestCommentService_Create_CleansCopiedCommentImagesWhenRepositoryFails(t *testing.T) {
	store := newCommentAssetStore()
	store.keys["temp/comments/7/images/cat.jpg"] = true
	store.keyMap["https://cdn.example.com/blog/temp/comments/7/images/cat.jpg"] = "temp/comments/7/images/cat.jpg"
	repo := &fakeCommentRepo{createErr: errors.New("db down")}
	svc := commentservice.NewCommentService(repo, store, nil, nil, nil)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{
		Content: "![cat](https://cdn.example.com/blog/temp/comments/7/images/cat.jpg)",
	}, 7)

	require.EqualError(t, err, "db down")
	assert.Equal(t, []string{"comments/article/3/images/cat.jpg"}, store.deleted)
}

func TestCommentService_ToggleLike_InvalidID(t *testing.T) {
	svc := commentservice.NewCommentService(&fakeCommentRepo{}, nil, nil, nil, nil)

	_, err := svc.ToggleLike("article", 0, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentTargetInvalid)
}

func TestCommentService_ToggleLike_ReturnsLatestState(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleLikeResp: &commentrepo.LikeResult{IsLiked: true, LikeCount: 3},
	}
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

	resp, err := svc.ToggleLike("article", 9, 7)

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.toggleLikeTarget.Type)
	assert.Equal(t, uint(9), repo.toggleLikeCommentID)
	assert.Equal(t, uint(7), repo.toggleLikeUserID)
	assert.True(t, resp.IsLiked)
	assert.Equal(t, int64(3), resp.LikeCount)
}

func TestCommentService_ToggleReplyLike_PublishesReplyLikedEvent(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleReplyLikeResp: &commentrepo.LikeResult{
			IsLiked: true, LikeCount: 3, TargetUserID: 8, RootID: 3,
			CommentID:     9,
			TargetContent: "回复正文",
			RootSnapshot:  commentrepo.RootSnapshot{Type: "article", ID: 3, Title: "文章标题", Excerpt: "文章摘要"},
		},
	}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	resp, err := svc.ToggleReplyLike("article", 12, 7)

	require.NoError(t, err)
	assert.True(t, resp.IsLiked)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.toggleReplyLikeTarget.Type)
	assert.Equal(t, uint(12), repo.toggleReplyLikeID)
	assert.Equal(t, uint(7), repo.toggleReplyLikeUserID)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeReplyLiked, pub.events[0].Type)
	assert.Equal(t, "reply", pub.events[0].SourceType)
	assert.Equal(t, uint(12), pub.events[0].SourceID)
	assert.Equal(t, "article", pub.events[0].RootType)
	assert.Equal(t, uint(3), pub.events[0].RootID)
	require.NotNil(t, pub.events[0].ActorUserID)
	assert.Equal(t, uint(7), *pub.events[0].ActorUserID)
	require.NotNil(t, pub.events[0].Metadata)
	assert.Contains(t, *pub.events[0].Metadata, "recipient_user_ids")
	assert.Contains(t, *pub.events[0].Metadata, `"source_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"回复正文"`)
	assert.Contains(t, *pub.events[0].Metadata, `"root_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"title":"文章标题"`)
	assert.Contains(t, *pub.events[0].Metadata, `"comment_id":9`)
}

func TestCommentService_Create_RejectsBlankContent(t *testing.T) {
	svc := commentservice.NewCommentService(&fakeCommentRepo{}, nil, nil, nil, nil)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{
		Content: "  ",
	}, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentContentRequired)
}

func TestCommentService_Create_MapsClosedTarget(t *testing.T) {
	repo := &fakeCommentRepo{createErr: commentrepo.ErrTargetCommentClosed}
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{
		Content: "好文章",
	}, 7)

	require.ErrorIs(t, err, commentservice.ErrCommentClosed)
}

func TestCommentService_DeleteComment_AllowsAdminForceDelete(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

	_, err := svc.DeleteComment("article", 9, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.True(t, repo.deleteCommentForce)
}

func TestCommentService_DeleteReply_UsesTargetPrefix(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

	_, err := svc.DeleteReply("article", 12, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint8(commentrepo.TargetArticle), repo.deleteReplyTarget.Type)
	assert.Equal(t, uint(12), repo.deleteReplyID)
	assert.True(t, repo.deleteReplyForce)
}

func TestCommentService_List_MapsRepositoryErrors(t *testing.T) {
	repo := &fakeCommentRepo{listErr: errors.New("boom")}
	svc := commentservice.NewCommentService(repo, nil, nil, nil, nil)

	_, err := svc.List("article", 3, dto.CommentListReq{Page: 1, PageSize: 10}, nil)

	require.EqualError(t, err, "boom")
}

// recordingPublisher 记录发布的事件，用于断言业务变更是否发布通知。
type recordingPublisher struct {
	events []notificationservice.PublishEvent
}

func (p *recordingPublisher) Publish(_ context.Context, e notificationservice.PublishEvent) (*model.NotificationEvent, error) {
	p.events = append(p.events, e)
	return &model.NotificationEvent{}, nil
}

type commentAssetStore struct {
	keys    map[string]bool
	keyMap  map[string]string
	copies  []commentAssetCopy
	deleted []string
}

type commentAssetCopy struct {
	source string
	target string
}

func newCommentAssetStore() *commentAssetStore {
	return &commentAssetStore{
		keys:   map[string]bool{},
		keyMap: map[string]string{},
	}
}

func (s *commentAssetStore) ObjectURL(_ context.Context, objectName string) (string, error) {
	return "https://cdn.example.com/blog/" + objectName, nil
}

func (s *commentAssetStore) ObjectExists(_ context.Context, objectName string) (bool, error) {
	return s.keys[objectName], nil
}

func (s *commentAssetStore) PutObject(context.Context, string, []byte, string) error {
	return nil
}

func (s *commentAssetStore) DeleteObject(_ context.Context, objectName string) error {
	s.deleted = append(s.deleted, objectName)
	delete(s.keys, objectName)
	return nil
}

func (s *commentAssetStore) MoveObject(context.Context, string, string) error {
	return nil
}

func (s *commentAssetStore) CopyObject(_ context.Context, sourceName string, targetName string) error {
	s.copies = append(s.copies, commentAssetCopy{source: sourceName, target: targetName})
	s.keys[targetName] = true
	return nil
}

func (s *commentAssetStore) ObjectKey(value string) (string, error) {
	if key, ok := s.keyMap[value]; ok {
		return key, nil
	}
	value = strings.TrimLeft(strings.TrimSpace(value), "/")
	if strings.HasPrefix(value, "temp/comments/") || strings.HasPrefix(value, "comments/") {
		return value, nil
	}
	return "", storage.ErrExternalObjectURL
}

func TestCommentService_Create_PublishesCommentEvent(t *testing.T) {
	repo := &fakeCommentRepo{createResp: &commentrepo.CommentAggregate{
		Comment:      commentrepo.CommentRecord{ID: 9, UserID: 7, Content: "好文章"},
		RootSnapshot: commentrepo.RootSnapshot{Type: "article", ID: 3, Title: "文章标题", Excerpt: "文章摘要"},
	}}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{Content: "好文章"}, 7)

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeCommentCreated, pub.events[0].Type)
	assert.Equal(t, "article", pub.events[0].RootType)
	assert.Equal(t, uint(3), pub.events[0].RootID)
	assert.Equal(t, uint(9), pub.events[0].SourceID)
	require.NotNil(t, pub.events[0].ActorUserID)
	assert.Equal(t, uint(7), *pub.events[0].ActorUserID)
	require.NotNil(t, pub.events[0].Metadata)
	assert.Contains(t, *pub.events[0].Metadata, `"source_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"好文章"`)
	assert.Contains(t, *pub.events[0].Metadata, `"root_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"title":"文章标题"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"文章摘要"`)
}

func TestCommentService_Create_GuestbookSetsExplicitRecipient(t *testing.T) {
	repo := &fakeCommentRepo{createResp: &commentrepo.CommentAggregate{
		Comment: commentrepo.CommentRecord{ID: 9, UserID: 7, Content: "留言"},
	}}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	// 留言板评论 target.ID 即板主用户 ID，应写入显式接收人 metadata。
	_, err := svc.Create("guestbook", 1, dto.CommentCreateReq{Content: "留言"}, 7)

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeCommentCreated, pub.events[0].Type)
	assert.Equal(t, "guestbook", pub.events[0].RootType)
	assert.Equal(t, uint(9), pub.events[0].RootID)
	require.NotNil(t, pub.events[0].Metadata)
	assert.Contains(t, *pub.events[0].Metadata, "recipient_user_ids")
}

func TestCommentService_Reply_PublishesReplyEventWithRecipient(t *testing.T) {
	repo := &fakeCommentRepo{replyResp: &commentrepo.ReplyAggregate{
		Reply:         commentrepo.ReplyRecord{ID: 12, CommentID: 9, FromUserID: 7, ToUserID: 8, Content: "收到"},
		TargetID:      3,
		QuotedContent: "原评论正文",
		RootSnapshot:  commentrepo.RootSnapshot{Type: "article", ID: 3, Title: "文章标题", Excerpt: "文章摘要"},
	}}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.Reply("article", 9, dto.CommentReplyCreateReq{Content: "收到"}, 7)

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeReplyCreated, pub.events[0].Type)
	require.NotNil(t, pub.events[0].Metadata)
	// 被回复人 8 应出现在显式接收人列表。
	assert.Contains(t, *pub.events[0].Metadata, "recipient_user_ids")
	assert.Contains(t, *pub.events[0].Metadata, `"source_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"收到"`)
	assert.Contains(t, *pub.events[0].Metadata, `"root_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"title":"文章标题"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"文章摘要"`)
	assert.Contains(t, *pub.events[0].Metadata, `"quote_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"原评论正文"`)
	assert.Contains(t, *pub.events[0].Metadata, `"comment_id":9`)
}

// 回复事件 RootID 应为根对象 ID（article ID），而非 commentID。
func TestCommentService_Reply_RootIDIsTargetIDNotCommentID(t *testing.T) {
	repo := &fakeCommentRepo{replyResp: &commentrepo.ReplyAggregate{
		Reply:    commentrepo.ReplyRecord{ID: 12, CommentID: 9, FromUserID: 7, ToUserID: 8, Content: "收到"},
		TargetID: 3,
	}}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.Reply("article", 9, dto.CommentReplyCreateReq{Content: "收到"}, 7)

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, uint(3), pub.events[0].RootID)
}

// 自己评论自己的对象不产生通知事件。
func TestCommentService_Create_SelfCommentDoesNotPublish(t *testing.T) {
	repo := &fakeCommentRepo{createResp: &commentrepo.CommentAggregate{
		Comment:     commentrepo.CommentRecord{ID: 9, UserID: 7, Content: "自评"},
		OwnerUserID: 7,
	}}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.Create("article", 3, dto.CommentCreateReq{Content: "自评"}, 7)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 自己回复自己不产生通知事件。
func TestCommentService_Reply_SelfReplyDoesNotPublish(t *testing.T) {
	repo := &fakeCommentRepo{replyResp: &commentrepo.ReplyAggregate{
		Reply: commentrepo.ReplyRecord{ID: 12, CommentID: 9, FromUserID: 7, ToUserID: 7, Content: "自回"},
	}}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.Reply("article", 9, dto.CommentReplyCreateReq{Content: "自回"}, 7)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 自己点赞自己的回复不产生通知事件。
func TestCommentService_ToggleReplyLike_SelfLikeDoesNotPublish(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleReplyLikeResp: &commentrepo.LikeResult{
			IsLiked: true, LikeCount: 2, RootID: 3, TargetUserID: 7,
		},
	}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.ToggleReplyLike("article", 12, 7)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 一级评论被其他用户点赞时发布 comment_liked 事件，接收人为评论作者。
func TestCommentService_ToggleLike_PublishesCommentLikedEvent(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleLikeResp: &commentrepo.LikeResult{
			IsLiked: true, LikeCount: 3, TargetUserID: 8, RootID: 5,
			TargetContent: "碎语评论正文",
			RootSnapshot:  commentrepo.RootSnapshot{Type: "moment", ID: 5, Excerpt: "碎语正文"},
		},
	}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	resp, err := svc.ToggleLike("moment", 12, 7)

	require.NoError(t, err)
	assert.True(t, resp.IsLiked)
	assert.Equal(t, uint8(commentrepo.TargetMoment), repo.toggleLikeTarget.Type)
	assert.Equal(t, uint(12), repo.toggleLikeCommentID)
	assert.Equal(t, uint(7), repo.toggleLikeUserID)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeCommentLiked, pub.events[0].Type)
	assert.Equal(t, "comment", pub.events[0].SourceType)
	assert.Equal(t, uint(12), pub.events[0].SourceID)
	assert.Equal(t, "moment", pub.events[0].RootType)
	assert.Equal(t, uint(5), pub.events[0].RootID)
	require.NotNil(t, pub.events[0].ActorUserID)
	assert.Equal(t, uint(7), *pub.events[0].ActorUserID)
	require.NotNil(t, pub.events[0].Metadata)
	assert.Contains(t, *pub.events[0].Metadata, "recipient_user_ids")
	assert.Contains(t, *pub.events[0].Metadata, `"source_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"碎语评论正文"`)
	assert.Contains(t, *pub.events[0].Metadata, `"root_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"碎语正文"`)
}

// 自己点赞自己的评论不产生通知事件。
func TestCommentService_ToggleLike_SelfLikeDoesNotPublish(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleLikeResp: &commentrepo.LikeResult{IsLiked: true, LikeCount: 2, RootID: 5, TargetUserID: 7},
	}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.ToggleLike("moment", 12, 7)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 取消评论点赞时不发布通知事件。
func TestCommentService_ToggleLike_DoesNotPublishOnUnlike(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleLikeResp: &commentrepo.LikeResult{IsLiked: false, LikeCount: 1, TargetUserID: 8, RootID: 5},
	}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.ToggleLike("moment", 12, 7)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 精确匹配用户场景：A(1) 的碎语(10)，A(1) 自己评论(20)，C(2) 点赞 A 的评论。
// comment_liked 的接收人通过 metadata 显式指定为评论作者(A)，与碎语作者无关。
func TestCommentService_ToggleLike_SelfMomentSelfCommentOtherLike_Publishes(t *testing.T) {
	repo := &fakeCommentRepo{
		toggleLikeResp: &commentrepo.LikeResult{
			IsLiked:      true,
			LikeCount:    1,
			TargetUserID: 1,  // 评论作者 = A(1)
			RootID:       10, // A(1) 的碎语
		},
	}
	pub := &recordingPublisher{}
	svc := commentservice.NewCommentService(repo, nil, pub, nil, nil)

	_, err := svc.ToggleLike("moment", 20, 2) // C(2) 点赞

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeCommentLiked, pub.events[0].Type)
	assert.Equal(t, "moment", pub.events[0].RootType)
	assert.Equal(t, uint(10), pub.events[0].RootID)
	require.NotNil(t, pub.events[0].ActorUserID)
	assert.Equal(t, uint(2), *pub.events[0].ActorUserID) // actor = C(2)
	require.NotNil(t, pub.events[0].Metadata)
	assert.Contains(t, *pub.events[0].Metadata, `"recipient_user_ids":[1]`) // 接收人 = A(1)
}
