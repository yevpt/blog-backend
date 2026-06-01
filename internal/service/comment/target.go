package comment

import (
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/pkg/roles"
)

const (
	targetTypeArticle   = "article"
	targetTypeMoment    = "moment"
	targetTypeGuestbook = "guestbook"
)

func parseTarget(targetType string, targetID uint) (commentrepo.Target, error) {
	if targetID == 0 {
		return commentrepo.Target{}, ErrCommentTargetInvalid
	}

	switch targetType {
	case targetTypeArticle:
		return commentrepo.Target{Type: commentrepo.TargetArticle, ID: targetID}, nil
	case targetTypeMoment:
		return commentrepo.Target{Type: commentrepo.TargetMoment, ID: targetID}, nil
	case targetTypeGuestbook:
		return commentrepo.Target{Type: commentrepo.TargetGuestbook, ID: targetID}, nil
	default:
		return commentrepo.Target{}, ErrCommentTargetInvalid
	}
}

func parseTargetType(targetType string) (uint8, error) {
	target, err := parseTarget(targetType, 1)
	return target.Type, err
}

func targetTypeName(commentType uint8) string {
	switch commentType {
	case commentrepo.TargetArticle:
		return targetTypeArticle
	case commentrepo.TargetMoment:
		return targetTypeMoment
	case commentrepo.TargetGuestbook:
		return targetTypeGuestbook
	default:
		return ""
	}
}

func hasAdminRole(roleNames []string) bool {
	for _, roleName := range roleNames {
		if roleName == roles.AdminRole {
			return true
		}
	}
	return false
}
