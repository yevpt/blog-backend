package notification

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"
)

// BatchObjectDeleted 批量判断通知引用对象是否已删除；缺失行按已删除处理。
func (d *Directory) BatchObjectDeleted(ctx context.Context, refs []ObjectDeletedRef) (map[ObjectDeletedKey]bool, error) {
	out := make(map[ObjectDeletedKey]bool, len(refs))
	groups := make(map[string][]ObjectDeletedRef)
	for _, ref := range refs {
		key := objectDeletedKey(ref)
		out[key] = true
		table, ok := objectDeletedTable(ref)
		if !ok {
			continue
		}
		groups[table] = append(groups[table], ref)
	}

	for table, tableRefs := range groups {
		rows, err := d.deletedRows(ctx, table, objectDeletedIDs(tableRefs))
		if err != nil {
			return nil, err
		}
		for _, ref := range tableRefs {
			deletedAt, exists := rows[ref.ObjectID]
			if !exists {
				continue
			}
			out[objectDeletedKey(ref)] = deletedAt.Valid
		}
	}
	return out, nil
}

func objectDeletedTable(ref ObjectDeletedRef) (string, bool) {
	switch ref.ObjectType {
	case "article":
		return "article", true
	case "moment":
		return "moment", true
	case "guestbook":
		return "guestbook", true
	case "user":
		return "user", true
	case "comment":
		switch ref.RootType {
		case "article":
			return "article_comment", true
		case "moment":
			return "moment_comment", true
		case "guestbook":
			return "guestbook", true
		}
	case "reply":
		switch ref.RootType {
		case "article":
			return "article_comment_reply", true
		case "moment":
			return "moment_comment_reply", true
		case "guestbook":
			return "guestbook_reply", true
		}
	}
	return "", false
}

func (d *Directory) deletedRows(ctx context.Context, table string, ids []uint) (map[uint]sql.NullTime, error) {
	out := make(map[uint]sql.NullTime, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID        uint
		DeletedAt sql.NullTime
	}
	err := d.db.WithContext(ctx).Session(&gorm.Session{}).
		Table(table).
		Select("id, deleted_at").
		Where("id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("读取 %s 删除状态: %w", table, err)
	}
	for _, row := range rows {
		out[row.ID] = row.DeletedAt
	}
	return out, nil
}

func objectDeletedIDs(refs []ObjectDeletedRef) []uint {
	seen := make(map[uint]struct{}, len(refs))
	ids := make([]uint, 0, len(refs))
	for _, ref := range refs {
		if ref.ObjectID == 0 {
			continue
		}
		if _, ok := seen[ref.ObjectID]; ok {
			continue
		}
		seen[ref.ObjectID] = struct{}{}
		ids = append(ids, ref.ObjectID)
	}
	return ids
}

func objectDeletedKey(ref ObjectDeletedRef) ObjectDeletedKey {
	return ObjectDeletedKey{
		ObjectType: ref.ObjectType,
		ObjectID:   ref.ObjectID,
		RootType:   ref.RootType,
	}
}
