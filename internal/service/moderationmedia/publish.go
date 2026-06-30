// Package moderationmedia 的图片正式化编排将审核通过图片从暂存复制到正式路径并补偿跨资源失败。
package moderationmedia

import (
	"context"
	"errors"
	"path"
	"strconv"
	"strings"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
)

// PublishCommand 描述一次审核通过图片正式化命令。
type PublishCommand struct {
	ItemID     uint64
	RevisionID uint64
	UserID     uint64
	MomentID   uint64
	Current    []moderationrepo.RevisionImageRecord
	Previous   []moderationrepo.RevisionImageRecord
}

// PublishedImage 记录单张图片的正式化结果。
type PublishedImage struct {
	SourceKey string
	PublicKey string
	Seq       uint
}

// PublishResult 汇总正式化结果。
type PublishResult struct {
	Images     []PublishedImage
	AuditMoves map[string]string
}

// PublishRegistry 是正式化所需的审核数据原子更新边界。
type PublishRegistry interface {
	ApplyPublishedImageKeys(ctx context.Context, cmd moderationrepo.PublishedImageCommand) error
}

// publishObjectStore 是正式化编排所需的最小对象存储能力。
type publishObjectStore interface {
	CopyObject(ctx context.Context, sourceName, targetName string) error
	ObjectExists(ctx context.Context, objectName string) (bool, error)
	DeleteObject(ctx context.Context, objectName string) error
}

// Publisher 将审核通过图片从暂存复制到正式路径，并编排数据库与对象的补偿。
type Publisher interface {
	Publish(ctx context.Context, cmd PublishCommand) (PublishResult, error)
}

type publisher struct {
	store    publishObjectStore
	registry PublishRegistry
}

// NewPublisher 通过构造注入创建图片正式化编排器。
func NewPublisher(store publishObjectStore, registry PublishRegistry) Publisher {
	return &publisher{store: store, registry: registry}
}

// Publish 执行图片正式化：先计算纯计划，再复制对象，全部成功后更新数据库引用；
// 数据库失败时删除本轮新正式对象，成功后删除暂存和已转存的旧公开对象。
func (p *publisher) Publish(ctx context.Context, cmd PublishCommand) (PublishResult, error) {
	if p == nil || p.store == nil || p.registry == nil {
		return PublishResult{}, ErrImageUnavailable
	}
	if cmd.ItemID == 0 || cmd.RevisionID == 0 || cmd.UserID == 0 || cmd.MomentID == 0 {
		return PublishResult{}, ErrInvalidImage
	}
	plan := computePublishPlan(cmd)
	// 复制暂存到正式和旧公开到审计。
	newKeys := make([]string, 0, len(plan.Images))
	for _, img := range plan.Images {
		if img.SourceKey == img.PublicKey {
			continue
		}
		exists, err := p.store.ObjectExists(ctx, img.PublicKey)
		if err != nil {
			return PublishResult{}, errors.Join(ErrImageUnavailable, err)
		}
		if exists {
			continue
		}
		if err := p.store.CopyObject(ctx, img.SourceKey, img.PublicKey); err != nil {
			return PublishResult{}, errors.Join(ErrImageUnavailable, err)
		}
		newKeys = append(newKeys, img.PublicKey)
	}
	newAuditKeys := make([]string, 0, len(plan.AuditMoves))
	for old, audit := range plan.AuditMoves {
		exists, err := p.store.ObjectExists(ctx, audit)
		if err != nil {
			p.compensateCopies(ctx, newKeys)
			return PublishResult{}, errors.Join(ErrImageUnavailable, err)
		}
		if exists {
			continue
		}
		if err := p.store.CopyObject(ctx, old, audit); err != nil {
			p.compensateCopies(ctx, newKeys)
			return PublishResult{}, errors.Join(ErrImageUnavailable, err)
		}
		newAuditKeys = append(newAuditKeys, audit)
	}
	// 全部复制成功后更新数据库引用。
	repoCmd := moderationrepo.PublishedImageCommand{
		ItemID: cmd.ItemID, RevisionID: cmd.RevisionID,
		MomentID: cmd.MomentID, AuthorID: cmd.UserID,
	}
	for _, img := range plan.Images {
		repoCmd.ImageKeys = append(repoCmd.ImageKeys, moderationrepo.PublishedImageKey{
			Seq: img.Seq, ObjectKey: img.PublicKey,
		})
	}
	for old, audit := range plan.AuditMoves {
		repoCmd.AuditMoves = append(repoCmd.AuditMoves, moderationrepo.AuditImageMove{
			OldObjectKey: old, NewObjectKey: audit,
		})
	}
	if err := p.registry.ApplyPublishedImageKeys(ctx, repoCmd); err != nil {
		// 数据库失败：删除本轮新复制的正式和审计对象。
		p.compensateCopies(ctx, append(newKeys, newAuditKeys...))
		return PublishResult{}, err
	}
	// 数据库成功：删除暂存和已转存的旧公开对象。
	p.cleanupSourceObjects(ctx, plan)
	return plan, nil
}

// compensateCopies 删除本轮新创建的对象，补偿数据库失败。
func (p *publisher) compensateCopies(ctx context.Context, keys []string) {
	for _, key := range keys {
		_ = p.store.DeleteObject(ctx, key)
	}
}

// cleanupSourceObjects 删除暂存和已转存审计的旧公开对象。
func (p *publisher) cleanupSourceObjects(ctx context.Context, plan PublishResult) {
	for _, img := range plan.Images {
		if img.SourceKey != img.PublicKey && isStagingKey(img.SourceKey) {
			_ = p.store.DeleteObject(ctx, img.SourceKey)
		}
	}
	for old := range plan.AuditMoves {
		_ = p.store.DeleteObject(ctx, old)
	}
}

// publishPlan 计算纯正式化计划，不触碰对象存储。
type publishPlanEntry struct {
	source string
	public string
	seq    uint
}

func computePublishPlan(cmd PublishCommand) PublishResult {
	images := make([]PublishedImage, 0, len(cmd.Current))
	currentHashes := make(map[string]struct{}, len(cmd.Current))
	for _, img := range cmd.Current {
		currentHashes[img.SHA256] = struct{}{}
		publicKey := momentFormalKey(cmd.UserID, cmd.MomentID, img.SHA256, img.MediaType)
		images = append(images, PublishedImage{
			SourceKey: img.ObjectKey, PublicKey: publicKey, Seq: img.Seq,
		})
	}
	auditMoves := make(map[string]string)
	for _, img := range cmd.Previous {
		if _, kept := currentHashes[img.SHA256]; kept {
			continue
		}
		if !isFormalMomentKey(img.ObjectKey) {
			continue
		}
		audit := momentAuditKey(cmd.ItemID, img.SHA256, img.MediaType)
		auditMoves[img.ObjectKey] = audit
	}
	return PublishResult{Images: images, AuditMoves: auditMoves}
}

// momentFormalKey 生成碎语图片正式路径 moments/{userID}/{momentID}/{hash}.{ext}。
func momentFormalKey(userID, momentID uint64, sha, mediaType string) string {
	return path.Join("moments", strconv.FormatUint(userID, 10), strconv.FormatUint(momentID, 10), sha+mediaExt(mediaType))
}

// momentAuditKey 生成碎语图片私有审计路径 moderation/history/moments/{itemID}/{hash}.{ext}。
func momentAuditKey(itemID uint64, sha, mediaType string) string {
	return path.Join("moderation/history/moments", strconv.FormatUint(itemID, 10), sha+mediaExt(mediaType))
}

func mediaExt(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return path.Ext(mediaType)
	}
}

func isStagingKey(key string) bool {
	return strings.HasPrefix(key, "moderation/staging/") || strings.HasPrefix(key, "comments/moderation/") || strings.HasPrefix(key, "temp/")
}

func isFormalMomentKey(key string) bool {
	return strings.HasPrefix(key, "moments/")
}
