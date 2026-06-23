package notification

import (
	"context"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// ObjectDeletedResolver 批量解析通知引用对象当前是否已删除。
type ObjectDeletedResolver interface {
	BatchObjectDeleted(ctx context.Context, refs []notificationrepo.ObjectDeletedRef) (map[notificationrepo.ObjectDeletedKey]bool, error)
}

func (s *inboxService) loadObjectDeletedStatus(ctx context.Context, items []notificationrepo.InboxAggregate) (map[notificationrepo.ObjectDeletedKey]bool, error) {
	if s.status == nil {
		return map[notificationrepo.ObjectDeletedKey]bool{}, nil
	}
	refs := objectDeletedRefs(items)
	if len(refs) == 0 {
		return map[notificationrepo.ObjectDeletedKey]bool{}, nil
	}
	return s.status.BatchObjectDeleted(ctx, refs)
}

func objectDeletedRefs(items []notificationrepo.InboxAggregate) []notificationrepo.ObjectDeletedRef {
	refs := make([]notificationrepo.ObjectDeletedRef, 0, len(items)*2)
	seen := make(map[notificationrepo.ObjectDeletedKey]struct{}, len(items)*2)
	add := func(ref notificationrepo.ObjectDeletedRef) {
		if ref.ObjectID == 0 || ref.ObjectType == "" || ref.ObjectType == "system" {
			return
		}
		key := objectDeletedKey(ref)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
	}
	for _, item := range items {
		add(sourceDeletedRef(item.Event))
		add(rootDeletedRef(item.Event))
	}
	return refs
}

func sourceDeletedRef(event model.NotificationEvent) notificationrepo.ObjectDeletedRef {
	return notificationrepo.ObjectDeletedRef{
		ObjectType: event.SourceType,
		ObjectID:   event.SourceID,
		RootType:   event.RootType,
	}
}

func rootDeletedRef(event model.NotificationEvent) notificationrepo.ObjectDeletedRef {
	objectType := event.RootType
	if event.SourceType == "comment" && event.RootType == "guestbook" {
		objectType = "user"
	}
	return notificationrepo.ObjectDeletedRef{
		ObjectType: objectType,
		ObjectID:   event.RootID,
	}
}

func sourceDeletedKey(event model.NotificationEvent) notificationrepo.ObjectDeletedKey {
	return objectDeletedKey(sourceDeletedRef(event))
}

func rootDeletedKey(event model.NotificationEvent) notificationrepo.ObjectDeletedKey {
	return objectDeletedKey(rootDeletedRef(event))
}

func objectDeletedKey(ref notificationrepo.ObjectDeletedRef) notificationrepo.ObjectDeletedKey {
	return notificationrepo.ObjectDeletedKey{
		ObjectType: ref.ObjectType,
		ObjectID:   ref.ObjectID,
		RootType:   ref.RootType,
	}
}
