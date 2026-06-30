package dbschema

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/roles"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultAdminPassword = "admin"

type SeedOptions struct {
	AdminUsername string
	AdminPassword string
	AdminEmail    string
}

type defaultSeedData struct {
	Roles                  []roleSeed
	Users                  []model.User
	UserRoles              []userRoleSeed
	UserMetas              []model.UserMeta
	UserSettings           []model.UserSetting
	EmailQuotaPolicies     []model.EmailQuotaPolicy
	EmailRoleQuotaPolicies []model.EmailRoleQuotaPolicy
	ModerationRuleSources  []model.ModerationRuleSource
	ModerationRulesets     []model.ModerationRuleset
	ModerationRules        []model.ModerationRule
	ModerationControls     []model.ModerationControl
}

type roleSeed struct {
	ID   uint
	Name string
}

type userRoleSeed struct {
	UserID uint
	RoleID uint
}

// SeedDefaults 写入当前系统启动所需的基础数据，重复执行不会覆盖已有记录。
func SeedDefaults(db *gorm.DB, opts SeedOptions) error {
	password := strings.TrimSpace(opts.AdminPassword)
	if password == "" {
		password = defaultAdminPassword
	}

	hash, err := hashAdminPassword(password)
	if err != nil {
		return err
	}

	data := buildDefaultSeedData(hash, opts)
	if err := insertRoleSeeds(db, data.Roles); err != nil {
		return err
	}
	if err := insertSeeds(db, data.Users, "user"); err != nil {
		return err
	}
	if err := insertUserRoleSeeds(db, data.UserRoles); err != nil {
		return err
	}
	if err := insertSeeds(db, data.UserMetas, "user_meta"); err != nil {
		return err
	}
	if err := insertSeeds(db, data.UserSettings, "user_setting"); err != nil {
		return err
	}
	if err := insertSeeds(db, data.EmailQuotaPolicies, "email_quota_policy"); err != nil {
		return err
	}
	if err := insertSeeds(db, data.EmailRoleQuotaPolicies, "email_role_quota_policy"); err != nil {
		return err
	}
	if err := insertSeeds(db, data.ModerationRuleSources, "moderation_rule_source"); err != nil {
		return err
	}
	if err := insertSeeds(db, data.ModerationRulesets, "moderation_ruleset"); err != nil {
		return err
	}
	if err := insertSeeds(db, data.ModerationRules, "moderation_rule"); err != nil {
		return err
	}
	return insertSeeds(db, data.ModerationControls, "moderation_control")
}

func hashAdminPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("生成 admin 密码 hash: %w", err)
	}
	return string(hash), nil
}

func buildDefaultSeedData(adminPasswordHash string, opts SeedOptions) defaultSeedData {
	username := strings.TrimSpace(opts.AdminUsername)
	if username == "" {
		username = "admin"
	}

	var email *string
	if value := strings.TrimSpace(opts.AdminEmail); value != "" {
		email = &value
	}

	return defaultSeedData{
		Roles: []roleSeed{
			{ID: roles.AdminRoleId, Name: roles.AdminRole},
			{ID: roles.NormalRoleId, Name: roles.NormalRole},
			{ID: roles.VipRoleId, Name: roles.VipRole},
		},
		Users: []model.User{
			{
				Base:        model.Base{ID: 1},
				Username:    username,
				Password:    adminPasswordHash,
				PasswordSet: true,
				Email:       email,
				Status:      1,
			},
		},
		UserRoles: []userRoleSeed{
			{UserID: 1, RoleID: roles.AdminRoleId},
		},
		UserMetas: []model.UserMeta{
			{UserID: 1},
		},
		UserSettings: []model.UserSetting{
			{UserID: 1, ReceiveMail: true, ShowAge: true, ShowPosition: true},
		},
		EmailQuotaPolicies: []model.EmailQuotaPolicy{
			{Purpose: "register_code", DailyLimit: 200, ReservedMin: 50, Priority: 1, MaxPerMinute: 5, MaxPerHour: 100, Enabled: true},
			{Purpose: "password_reset", DailyLimit: 200, ReservedMin: 30, Priority: 1, MaxPerMinute: 5, MaxPerHour: 100, Enabled: true},
			{Purpose: "security", DailyLimit: 100, ReservedMin: 10, Priority: 5, MaxPerMinute: 5, MaxPerHour: 60, Enabled: true},
			{Purpose: "notification", DailyLimit: 150, ReservedMin: 0, Priority: 100, MaxPerMinute: 5, MaxPerHour: 80, Enabled: true},
			{Purpose: "admin_notice", DailyLimit: 100, ReservedMin: 0, Priority: 50, MaxPerMinute: 5, MaxPerHour: 60, Enabled: true},
		},
		EmailRoleQuotaPolicies: []model.EmailRoleQuotaPolicy{
			{Role: "normal", ScopeType: "actor", DailyLimit: 30, MaxPerHour: 0, Enabled: true},
			{Role: "vip", ScopeType: "actor", DailyLimit: 100, MaxPerHour: 0, Enabled: true},
			{Role: "admin", ScopeType: "actor", DailyLimit: 300, MaxPerHour: 0, Enabled: true},
			{Role: "normal", ScopeType: "recipient", DailyLimit: 5, MaxPerHour: 0, Enabled: true},
			{Role: "vip", ScopeType: "recipient", DailyLimit: 20, MaxPerHour: 0, Enabled: true},
			{Role: "admin", ScopeType: "recipient", DailyLimit: 50, MaxPerHour: 0, Enabled: true},
		},
		ModerationRuleSources: []model.ModerationRuleSource{
			{ID: 1, Name: "system"},
		},
		ModerationRulesets: []model.ModerationRuleset{
			{
				ID:                 1,
				Status:             "published",
				RuleCount:          1,
				KeywordCount:       1,
				IndexFormatVersion: 1,
			},
		},
		ModerationRules: []model.ModerationRule{
			{
				ID:                 1,
				Name:               seedString("礼貌用语基线"),
				RuleType:           model.ModerationRuleKeyword,
				Pattern:            "谢谢",
				DedupeHash:         moderationRuleDedupeHash("review", model.ModerationRuleKeyword, "谢谢"),
				Category:           "other",
				Effect:             "review",
				RiskLevel:          model.ModerationRiskLow,
				Priority:           1000,
				SourceID:           1,
				ActivatedRulesetID: 1,
			},
		},
		ModerationControls: []model.ModerationControl{
			{
				ID:               1,
				RegistrationMode: model.ModerationRegistrationOpen,
				PublishingMode:   model.ModerationPublishingOpen,
				LockVersion:      1,
			},
		},
	}
}

func moderationRuleDedupeHash(effect, ruleType, pattern string) []byte {
	hash := sha256.Sum256([]byte(effect + "\x00" + ruleType + "\x00" + pattern))
	return hash[:]
}

func seedString(value string) *string {
	return &value
}

func insertRoleSeeds(db *gorm.DB, seeds []roleSeed) error {
	models := make([]model.Role, 0, len(seeds))
	for _, seed := range seeds {
		models = append(models, model.Role{ID: seed.ID, Name: seed.Name})
	}
	return insertSeeds(db, models, "role")
}

func insertUserRoleSeeds(db *gorm.DB, seeds []userRoleSeed) error {
	models := make([]model.UserRole, 0, len(seeds))
	for _, seed := range seeds {
		models = append(models, model.UserRole{UserID: seed.UserID, RoleID: seed.RoleID})
	}
	return insertSeeds(db, models, "user_role")
}

func insertSeeds[T any](db *gorm.DB, values []T, table string) error {
	if len(values) == 0 {
		return nil
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&values).Error; err != nil {
		return fmt.Errorf("写入默认数据 %s: %w", table, err)
	}
	return nil
}
