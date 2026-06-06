// 数据迁移工具：将旧数据库（blog）的数据按优化后的表结构迁移到新库（blog_dev）。
//
// 源库特点：Java 风格命名（大写 ID）、varchar 状态字段（'01'/'00'）、
//
//	无软删除字段、date_create/date_modifed 命名、utf8mb3 字符集。
//
// 迁移按依赖顺序执行，每步幂等——目标表已有数据则跳过（可用 --force 强制重跑）。
//
// 用法：
//
//	SRC_DSN="blog:_%Lryyld527%_@tcp(d.yevpt.com:9003)/blog?charset=utf8mb4&parseTime=True&loc=Local" \
//	  go run ./cmd/migrate/ [--force]
//
// 目标库从项目 config.local.yaml 的 db 配置读取。
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/database"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────
// 程序入口
// ─────────────────────────────────────────────

func main() {
	force := len(os.Args) > 1 && os.Args[1] == "--force"

	// 1. 加载配置（源库 DSN 和目标库连接信息均从 config.local.yaml 读取）
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败（请确保 config/config.local.yaml 存在）: %v", err)
	}

	// 2. 连接源库（原始 blog 库，只读）
	// DSN 优先读取环境变量 SRC_DSN，其次读 config.local.yaml 的 migrate.src_dsn
	srcDSN := os.Getenv("SRC_DSN")
	if srcDSN == "" {
		srcDSN = cfg.Migrate.SrcDSN
	}
	if srcDSN == "" {
		log.Fatalf("源库 DSN 未配置，请在 config.local.yaml 设置 migrate.src_dsn 或设置 SRC_DSN 环境变量")
	}
	src, err := sql.Open("mysql", srcDSN)
	if err != nil {
		log.Fatalf("源库连接失败: %v", err)
	}
	defer src.Close()
	if err := src.Ping(); err != nil {
		log.Fatalf("源库 ping 失败: %v", err)
	}
	log.Println("✓ 源库连接成功")

	// 3. 连接目标库（新 blog_dev 库，读写）
	dst, err := database.NewMySQL(&cfg.DB)
	if err != nil {
		log.Fatalf("目标库连接失败: %v", err)
	}
	log.Printf("✓ 目标库连接成功（%s/%s）", cfg.DB.Host, cfg.DB.Name)

	// 3. 在目标库自动建表（按新模型结构）
	log.Println("→ AutoMigrate 建表...")
	if err := autoMigrate(dst); err != nil {
		log.Fatalf("AutoMigrate 失败: %v", err)
	}
	log.Println("✓ 建表完成")

	// 4. 按依赖顺序依次执行迁移步骤
	steps := []struct {
		name string
		fn   func(*sql.DB, *gorm.DB) error
	}{
		{"Step 01: role", migrateRole},
		{"Step 02: user", migrateUser},
		{"Step 03: user_role", migrateUserRole},
		{"Step 04: user_meta + user_social_link", migrateUserMeta},
		{"Step 05: user_setting", migrateUserSetting},
		{"Step 06: social_user", migrateSocialUser},
		{"Step 07: social_user_auth", migrateSocialUserAuth},
		{"Step 08: category", migrateCategory},
		{"Step 09: tag", migrateTag},
		{"Step 10: music", migrateMusic},
		{"Step 11: post → article", migrateArticle},
		{"Step 12: say → moment", migrateMoment},
		{"Step 13: link → friend_link", migrateFriendLink},
		{"Step 14: media", migrateMedia},
		{"Step 15: category_post → article_category", migrateArticleCategory},
		{"Step 16: tag_post → article_tag", migrateArticleTag},
		{"Step 17: post_music → article_music", migrateArticleMusic},
		{"Step 18: recommend_post → article_recommend", migrateArticleRecommend},
		{"Step 19: comment → article_comment/moment_comment/guestbook", migrateComments},
		{"Step 20: comment_reply", migrateCommentReply},
		{"Step 21: post_like → user_like", migrateUserLike},
		{"Step 22: message", migrateMessage},
		{"Step 23: user_messages → user_message", migrateUserMessage},
	}

	for _, step := range steps {
		log.Printf("→ %s", step.name)
		// 幂等检查：目标表若已有数据则跳过（--force 时强制执行）
		if !force && hasData(dst, targetTableForStep(step.name)) {
			log.Printf("  跳过（目标表已有数据，使用 --force 强制重跑）")
			continue
		}
		if err := step.fn(src, dst); err != nil {
			log.Fatalf("  ✗ %s 失败: %v", step.name, err)
		}
		log.Printf("  ✓ 完成")
	}

	// 5. 迁移后完整性清理（无论 force 与否，每次都执行）
	// 源库中存在被攻击产生的垃圾数据以及历史软删除导致的孤儿记录，需在迁移后统一清理。
	log.Println("→ Step 24: 完整性清理（孤儿记录）")
	if err := cleanOrphans(dst); err != nil {
		log.Fatalf("  ✗ 完整性清理失败: %v", err)
	}
	log.Printf("  ✓ 完成")

	// 6. ID 整理：压缩各表因攻击/历史删除产生的 ID 间隙，重置 AUTO_INCREMENT（每次都执行）
	log.Println("→ Step 25: ID 整理（压缩 ID 间隙 + 重置 AUTO_INCREMENT）")
	if err := defragIDs(dst); err != nil {
		log.Fatalf("  ✗ ID 整理失败: %v", err)
	}
	log.Printf("  ✓ 完成")

	// 7. 清理迁移过程中遗留的旧表，确保目标库结构与当前代码一致
	log.Println("→ Step 26: 清理旧表")
	if err := dropLegacyTables(dst); err != nil {
		log.Fatalf("  ✗ 清理旧表失败: %v", err)
	}
	log.Printf("  ✓ 完成")

	log.Println("\n✓ 全部迁移步骤完成")
}

// ─────────────────────────────────────────────
// AutoMigrate：注册所有目标模型
// ─────────────────────────────────────────────

func autoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Role{},
		&model.User{},
		&model.UserRole{},
		&model.UserLike{},
		&model.UserMeta{},
		&model.UserSetting{},
		&model.UserSocialLink{},
		&model.SocialUser{},
		&model.SocialUserAuth{},
		&model.Article{},
		&model.ArticleRecommend{},
		&model.ArticleCategory{},
		&model.ArticleTag{},
		&model.ArticleMusic{},
		&model.Category{},
		&model.Tag{},
		&model.Music{},
		&model.Moment{},
		&model.FriendLink{},
		&model.Media{},
		&model.ArticleComment{},
		&model.MomentComment{},
		&model.Guestbook{},
		&model.ArticleCommentReply{},
		&model.MomentCommentReply{},
		&model.GuestbookReply{},
		&model.Message{},
		&model.UserMessage{},
	)
}

// ─────────────────────────────────────────────
// 工具函数
// ─────────────────────────────────────────────

// hasData 检查目标表是否已有数据（幂等保护）
func hasData(db *gorm.DB, table string) bool {
	if table == "" {
		return false
	}
	var cnt int64
	db.Raw("SELECT COUNT(*) FROM `" + table + "`").Scan(&cnt)
	return cnt > 0
}

// targetTableForStep 根据步骤名称返回主目标表名（用于幂等检查）
func targetTableForStep(name string) string {
	m := map[string]string{
		"Step 01": "role",
		"Step 02": "user",
		"Step 03": "user_role",
		"Step 04": "user_meta",
		"Step 05": "user_setting",
		"Step 06": "social_user",
		"Step 07": "social_user_auth",
		"Step 08": "category",
		"Step 09": "tag",
		"Step 10": "music",
		"Step 11": "article",
		"Step 12": "moment",
		"Step 13": "friend_link",
		"Step 14": "media",
		"Step 15": "article_category",
		"Step 16": "article_tag",
		"Step 17": "article_music",
		"Step 18": "article_recommend",
		"Step 19": "article_comment",
		"Step 20": "article_comment_reply",
		"Step 21": "user_like",
		"Step 22": "message",
		"Step 23": "user_message",
		// Step 24 无独立目标表，每次都执行，不纳入幂等检查
	}
	prefix := name[:7] // "Step XX"
	return m[prefix]
}

// nullStr 将 sql.NullString 转为 *string
func nullStr(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	return &ns.String
}

// nullTime 将 sql.NullTime 转为 *time.Time
func nullTime(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}

// isUseToBool '01' = true（使用中），'00' = false（已停用）
func isUseToBool(s sql.NullString) bool {
	return s.Valid && s.String == "01"
}

// isUseToDeletedAt '00' 表示已停用 → 设为软删除时间；'01' 表示正常 → nil
func isUseToDeletedAt(s sql.NullString) *time.Time {
	if s.Valid && s.String == "00" {
		t := time.Now()
		return &t
	}
	return nil
}

// statusVarcharToUint8 将源库 varchar 状态值转为 tinyint
// 支持：'01'→1, '00'→0, 'on'→1, 'off'→0, 空/NULL→1（默认正常）
func statusVarcharToUint8(s sql.NullString) uint8 {
	if !s.Valid {
		return 1
	}
	switch s.String {
	case "00", "off", "0":
		return 0
	default:
		return 1
	}
}

// parseMusicDuration 将 "mm:ss" 格式的时长字符串转换为秒数
// 例如 "3:45" → 225，"1:02:30" → 3750（小时:分:秒），无效格式返回 0
func parseMusicDuration(s sql.NullString) uint16 {
	if !s.Valid || s.String == "" {
		return 0
	}
	parts := strings.Split(s.String, ":")
	switch len(parts) {
	case 2:
		m, _ := strconv.Atoi(parts[0])
		sec, _ := strconv.Atoi(parts[1])
		return uint16(m*60 + sec)
	case 3:
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		sec, _ := strconv.Atoi(parts[2])
		return uint16(h*3600 + m*60 + sec)
	default:
		return 0
	}
}

// parseSongDate 尝试将 varchar 日期字符串解析为 time.Time
// 支持格式："YYYYMMDD", "YYYY-MM-DD", "YYYY/MM/DD", "YYYY"
func parseSongDate(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	formats := []string{"20060102", "2006-01-02", "2006/01/02", "2006"}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, strings.TrimSpace(s.String), time.Local); err == nil {
			return &t
		}
	}
	return nil
}

// remapMediaOwnerType 将旧系统 media.owner_type 映射到新系统语义。
// 旧系统：1=say（碎语）；新系统：1=文章 2=说说 3=用户。
func remapMediaOwnerType(old int) uint8 {
	switch old {
	case 1: // 旧系统 say = 新系统 说说
		return 2
	default:
		return uint8(old)
	}
}

// nullUint 将 sql.NullInt64 转为 *uint，0 值也转为 nil
func nullUint(ni sql.NullInt64) *uint {
	if !ni.Valid || ni.Int64 == 0 {
		return nil
	}
	v := uint(ni.Int64)
	return &v
}

// ─────────────────────────────────────────────
// Step 01: role → role
// ─────────────────────────────────────────────
// 角色表是静态配置（admin/vip/normal），直接复制 ID 和 name。
// 新表去掉了软删除字段，角色不需要逻辑删除。

func migrateRole(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query("SELECT ID, name FROM role ORDER BY ID")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id uint
		var name sql.NullString
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		r := model.Role{ID: id, Name: name.String}
		if err := dst.Create(&r).Error; err != nil {
			return fmt.Errorf("insert role id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 02: user → user
// ─────────────────────────────────────────────
// 字段映射：
//   name        → nickname（昵称，与 user_meta.name 真实姓名区分）
//   status      → status（varchar 'on'/'off' → tinyint 1/0）
//   login_time  → last_login_at
//   register_time → created_at（注册时间即创建时间）
//   email_check → 丢弃（功能已合并到 user_setting，迁移时默认不开启）
//   avatar_img_url → avatar_url

func migrateUser(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, username, password, name, register_time, login_time,
		       status, email, phone, avatar_img_url, site, mark
		FROM user ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id           uint
			username     string
			password     string
			name         sql.NullString
			registerTime sql.NullTime
			loginTime    sql.NullTime
			status       sql.NullString
			email        sql.NullString
			phone        sql.NullString
			avatarImgUrl sql.NullString
			site         sql.NullString
			mark         sql.NullString
		)
		if err := rows.Scan(&id, &username, &password, &name, &registerTime,
			&loginTime, &status, &email, &phone, &avatarImgUrl, &site, &mark); err != nil {
			return err
		}

		u := model.User{
			Base:        model.Base{ID: id},
			Username:    username,
			Password:    password,
			Nickname:    nullStr(name),
			Email:       nullStr(email),
			Phone:       nullStr(phone),
			Site:        nullStr(site),
			AvatarUrl:   nullStr(avatarImgUrl),
			Mark:        nullStr(mark),
			Status:      statusVarcharToUint8(status),
			LastLoginAt: nullTime(loginTime),
		}
		// 用 register_time 作为 created_at
		if registerTime.Valid {
			u.CreatedAt = registerTime.Time
		}

		if err := dst.Create(&u).Error; err != nil {
			return fmt.Errorf("insert user id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 03: user_role → user_role
// ─────────────────────────────────────────────

func migrateUserRole(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query("SELECT ID, user_id, role_id FROM user_role ORDER BY ID")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, userID, roleID uint
		if err := rows.Scan(&id, &userID, &roleID); err != nil {
			return err
		}
		r := model.UserRole{ID: id, UserID: userID, RoleID: roleID}
		if err := dst.Create(&r).Error; err != nil {
			return fmt.Errorf("insert user_role id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 04: user_meta → user_meta + user_social_link
// ─────────────────────────────────────────────
// 关键变更：
//   1. email 字段丢弃（user 表已有，避免重复）
//   2. show_name 字段移至 user_setting（迁移时在 step05 处理）
//   3. 9 个社交账号列（qq/wechat/bili/facebook/sina/github/gitee/zhihu）
//      → 各自生成一行 user_social_link 记录
//   4. gender: varchar '0'/'1' → tinyint 0/1
//   5. user_meta.ID 在源库就是 user.ID（非自增），直接作为 user_id

func migrateUserMeta(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, name, description, gender, birthday, id_card,
		       province, city, address, country,
		       qq, wechat, bili, facebook, sina, github, gitee, zhihu
		FROM user_meta ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			name        sql.NullString
			description sql.NullString
			gender      sql.NullString
			birthday    sql.NullTime
			idCard      sql.NullString
			province    sql.NullString
			city        sql.NullString
			address     sql.NullString
			country     sql.NullString
			qq          sql.NullString
			wechat      sql.NullString
			bili        sql.NullString
			facebook    sql.NullString
			sina        sql.NullString
			github      sql.NullString
			gitee       sql.NullString
			zhihu       sql.NullString
		)
		if err := rows.Scan(&id, &name, &description, &gender, &birthday, &idCard,
			&province, &city, &address, &country,
			&qq, &wechat, &bili, &facebook, &sina, &github, &gitee, &zhihu); err != nil {
			return err
		}

		// 转换 gender: varchar '0'='男', '1'='女' → uint8
		var genderVal *uint8
		if gender.Valid && gender.String != "" {
			g, _ := strconv.Atoi(gender.String)
			v := uint8(g)
			genderVal = &v
		}

		// country 默认值
		countryStr := "中国"
		if country.Valid && country.String != "" {
			countryStr = country.String
		}

		meta := model.UserMeta{
			UserID:      id,
			Name:        nullStr(name),
			Description: nullStr(description),
			Gender:      genderVal,
			Birthday:    nullTime(birthday),
			IdCard:      nullStr(idCard),
			Country:     &countryStr,
			Province:    nullStr(province),
			City:        nullStr(city),
			Address:     nullStr(address),
		}
		if err := dst.Create(&meta).Error; err != nil {
			return fmt.Errorf("insert user_meta id=%d: %w", id, err)
		}

		// 将非空社交账号迁移到 user_social_link
		socialLinks := []struct {
			platform string
			val      sql.NullString
		}{
			{"qq", qq},
			{"wechat", wechat},
			{"bili", bili},
			{"facebook", facebook},
			{"sina", sina},
			{"github", github},
			{"gitee", gitee},
			{"zhihu", zhihu},
		}
		for _, sl := range socialLinks {
			if sl.val.Valid && sl.val.String != "" {
				link := model.UserSocialLink{
					UserID:   id,
					Platform: sl.platform,
					URL:      sl.val.String,
				}
				// 使用 Omit("id") 让 GORM 自动分配 ID
				if err := dst.Create(&link).Error; err != nil {
					return fmt.Errorf("insert user_social_link user=%d platform=%s: %w", id, sl.platform, err)
				}
			}
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 05: user_setting → user_setting
// ─────────────────────────────────────────────
// user_setting.ID 在源库就是 user.ID（非自增），直接作为 user_id。
// 源库无 show_age/show_facebook 字段，使用默认值。
// email_check 字段在源库属于 user 表（char '0'/'1'），这里统一默认 false。

func migrateUserSetting(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, mail_show, mail_receive, dark_mode, receive_mail,
		       show_name, show_age, show_phone, show_qq, show_wechat,
		       show_zhihu, show_sina, show_bili, show_position
		FROM user_setting ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			mailShow    sql.NullInt32
			mailReceive sql.NullInt32
			darkMode    sql.NullInt32
			receiveMail sql.NullInt32
			showName    sql.NullInt32
			showAge     sql.NullInt32
			showPhone   sql.NullInt32
			showQq      sql.NullInt32
			showWechat  sql.NullInt32
			showZhihu   sql.NullInt32
			showSina    sql.NullInt32
			showBili    sql.NullInt32
			showPos     sql.NullInt32
		)
		if err := rows.Scan(&id, &mailShow, &mailReceive, &darkMode, &receiveMail,
			&showName, &showAge, &showPhone, &showQq, &showWechat,
			&showZhihu, &showSina, &showBili, &showPos); err != nil {
			return err
		}

		intToBool := func(n sql.NullInt32) bool { return n.Valid && n.Int32 == 1 }
		intToUint8 := func(n sql.NullInt32) uint8 {
			if n.Valid {
				return uint8(n.Int32)
			}
			return 0
		}

		s := model.UserSetting{
			UserID:       id,
			MailShow:     intToUint8(mailShow),
			MailReceive:  intToUint8(mailReceive),
			DarkMode:     intToUint8(darkMode),
			ReceiveMail:  intToBool(receiveMail),
			ShowName:     intToBool(showName),
			ShowAge:      intToBool(showAge),
			ShowPhone:    intToBool(showPhone),
			ShowQq:       intToBool(showQq),
			ShowWechat:   intToBool(showWechat),
			ShowZhihu:    intToBool(showZhihu),
			ShowSina:     intToBool(showSina),
			ShowBili:     intToBool(showBili),
			ShowPosition: intToBool(showPos),
		}
		if err := dst.Create(&s).Error; err != nil {
			return fmt.Errorf("insert user_setting id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 06: social_user → social_user
// ─────────────────────────────────────────────
// is_use '00' → IsActive=false（不做软删除，社交账号状态用布尔字段）

func migrateSocialUser(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, uuid, source, access_token, refresh_token, open_id, is_use, date_create
		FROM social_user ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id           uint
			uuid         string
			source       string
			accessToken  sql.NullString
			refreshToken sql.NullString
			openID       sql.NullString
			isUse        sql.NullString
			dateCreate   sql.NullTime
		)
		if err := rows.Scan(&id, &uuid, &source, &accessToken, &refreshToken,
			&openID, &isUse, &dateCreate); err != nil {
			return err
		}

		su := model.SocialUser{
			Base:         model.Base{ID: id},
			UUID:         uuid,
			Source:       source,
			AccessToken:  accessToken.String,
			RefreshToken: nullStr(refreshToken),
			OpenID:       nullStr(openID),
			IsActive:     isUseToBool(isUse),
		}
		if dateCreate.Valid {
			su.CreatedAt = dateCreate.Time
		}
		if err := dst.Create(&su).Error; err != nil {
			return fmt.Errorf("insert social_user id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 07: social_user_auth → social_user_auth
// ─────────────────────────────────────────────
// is_use='00' 的记录表示已解绑，做软删除处理

func migrateSocialUserAuth(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query("SELECT ID, user_id, social_user_id, is_use FROM social_user_auth ORDER BY ID")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, userID, socialUserID uint
		var isUse sql.NullString
		if err := rows.Scan(&id, &userID, &socialUserID, &isUse); err != nil {
			return err
		}
		sa := model.SocialUserAuth{
			Base:         model.Base{ID: id},
			UserID:       userID,
			SocialUserID: socialUserID,
		}
		if isUse.Valid && isUse.String == "00" {
			// 软删除：直接设置 deleted_at
			t := time.Now()
			sa.DeletedAt = gorm.DeletedAt{Time: t, Valid: true}
		}
		if err := dst.Create(&sa).Error; err != nil {
			return fmt.Errorf("insert social_user_auth id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 08: category → category
// ─────────────────────────────────────────────
// is_use='00' → 软删除
// background_img_url → cover_img_url（字段改名）
// parent_Id（源库大写 I）→ parent_id

func migrateCategory(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, parent_Id, name, url, icon, description,
		       background_img_url, seq, date_create, date_modifed, is_use
		FROM category ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			parentID    sql.NullInt64
			name        sql.NullString
			url         sql.NullString
			icon        sql.NullString
			description sql.NullString
			bgImgUrl    sql.NullString
			seq         sql.NullInt32
			dateCreate  sql.NullTime
			dateModifed sql.NullTime
			isUse       sql.NullString
		)
		if err := rows.Scan(&id, &parentID, &name, &url, &icon, &description,
			&bgImgUrl, &seq, &dateCreate, &dateModifed, &isUse); err != nil {
			return err
		}

		c := model.Category{
			Base:        model.Base{ID: id},
			ParentID:    nullUint(parentID),
			Name:        name.String,
			URL:         nullStr(url),
			Icon:        nullStr(icon),
			Description: nullStr(description),
			CoverImgUrl: nullStr(bgImgUrl),
			Seq:         uint(seq.Int32),
		}
		if dateCreate.Valid {
			c.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			c.UpdatedAt = dateModifed.Time
		}
		if isUse.Valid && isUse.String == "00" {
			t := time.Now()
			c.DeletedAt = gorm.DeletedAt{Time: t, Valid: true}
		}
		if err := dst.Create(&c).Error; err != nil {
			return fmt.Errorf("insert category id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 09: tag → tag
// ─────────────────────────────────────────────

func migrateTag(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, name, url, icon, description, background_img_url, seq,
		       date_create, date_modifed, is_use
		FROM tag ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			name        sql.NullString
			url         sql.NullString
			icon        sql.NullString
			description sql.NullString
			bgImgUrl    sql.NullString
			seq         sql.NullInt32
			dateCreate  sql.NullTime
			dateModifed sql.NullTime
			isUse       sql.NullString
		)
		if err := rows.Scan(&id, &name, &url, &icon, &description, &bgImgUrl, &seq,
			&dateCreate, &dateModifed, &isUse); err != nil {
			return err
		}

		t := model.Tag{
			Base:        model.Base{ID: id},
			Name:        name.String,
			URL:         nullStr(url),
			Icon:        nullStr(icon),
			Description: nullStr(description),
			CoverImgUrl: nullStr(bgImgUrl),
			Seq:         uint(seq.Int32),
		}
		if dateCreate.Valid {
			t.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			t.UpdatedAt = dateModifed.Time
		}
		if isUse.Valid && isUse.String == "00" {
			now := time.Now()
			t.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
		}
		if err := dst.Create(&t).Error; err != nil {
			return fmt.Errorf("insert tag id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 10: music → music
// ─────────────────────────────────────────────
// 关键转换：
//   song_date varchar(15) → date（尝试解析 "YYYYMMDD" 等格式）
//   music_time varchar(10) → duration smallint（"mm:ss" → 秒）
//   icon → cover_img_url（字段改名，源库用 icon 存封面）
//   lyric → text 类型（容量不受限）

func migrateMusic(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, name, singer, album, song_date, url, icon,
		       description, lyric, seq, music_time,
		       date_create, date_modifed, is_use
		FROM music ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			name        sql.NullString
			singer      sql.NullString
			album       sql.NullString
			songDate    sql.NullString
			url         sql.NullString
			icon        sql.NullString
			description sql.NullString
			lyric       sql.NullString
			seq         sql.NullInt32
			musicTime   sql.NullString
			dateCreate  sql.NullTime
			dateModifed sql.NullTime
			isUse       sql.NullString
		)
		if err := rows.Scan(&id, &name, &singer, &album, &songDate, &url, &icon,
			&description, &lyric, &seq, &musicTime,
			&dateCreate, &dateModifed, &isUse); err != nil {
			return err
		}

		m := model.Music{
			Base:        model.Base{ID: id},
			Name:        name.String,
			Singer:      singer.String,
			Album:       album.String,
			SongDate:    parseSongDate(songDate),
			URL:         nullStr(url),
			CoverImgUrl: nullStr(icon),
			Description: nullStr(description),
			Lyric:       nullStr(lyric),
			Duration:    parseMusicDuration(musicTime),
			Seq:         uint(seq.Int32),
		}
		if dateCreate.Valid {
			m.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			m.UpdatedAt = dateModifed.Time
		}
		if isUse.Valid && isUse.String == "00" {
			t := time.Now()
			m.DeletedAt = gorm.DeletedAt{Time: t, Valid: true}
		}
		if err := dst.Create(&m).Error; err != nil {
			return fmt.Errorf("insert music id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 11: post → article
// ─────────────────────────────────────────────
// 丢弃的字段：
//   url           — 文章路由别名，新版用 id 做路由
//   html_content  — 富文本版本，新版只存 markdown，前端渲染
//   word_count    — 字数统计，可在运行时计算
//   type          — 源库全部为 null，无实际使用
//   parent_id     — 旧版遗留，文章归属通过 article_category 管理
// 封面图：优先取 background_img_url_phone，为空则取 background_img_url

func migrateArticle(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, title, user_id, content, short_content,
		       status, comment_status, password,
		       background_img_url, background_img_url_phone,
		       read_count, date_create, date_modifed
		FROM post ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id            uint
			title         sql.NullString
			userID        sql.NullInt64
			content       sql.NullString
			shortContent  sql.NullString
			status        sql.NullString
			commentStatus sql.NullString
			password      sql.NullString
			bgImgUrl      sql.NullString
			bgImgUrlPhone sql.NullString
			readCount     sql.NullInt32
			dateCreate    sql.NullTime
			dateModifed   sql.NullTime
		)
		if err := rows.Scan(&id, &title, &userID, &content, &shortContent,
			&status, &commentStatus, &password,
			&bgImgUrl, &bgImgUrlPhone,
			&readCount, &dateCreate, &dateModifed); err != nil {
			return err
		}

		// 封面图优先取手机版，为空则取桌面版
		coverImgUrl := nullStr(bgImgUrlPhone)
		if coverImgUrl == nil {
			coverImgUrl = nullStr(bgImgUrl)
		}

		a := model.Article{
			Base:          model.Base{ID: id},
			Title:         title.String,
			CoverImgUrl:   coverImgUrl,
			ShortContent:  nullStr(shortContent),
			Content:       content.String,
			UserID:        uint(userID.Int64),
			Status:        statusVarcharToUint8(status),
			CommentStatus: statusVarcharToUint8(commentStatus),
			Password:      nullStr(password),
			ReadCount:     uint(readCount.Int32),
		}
		if dateCreate.Valid {
			a.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			a.UpdatedAt = dateModifed.Time
		}
		if err := dst.Create(&a).Error; err != nil {
			return fmt.Errorf("insert article id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 12: say → moment
// ─────────────────────────────────────────────
// 丢弃的字段：title, short_content（说说不需要标题/摘要）
// IS_TOP 源库为 int → is_top bool
// status '01'→1（公开），'00'→0（隐藏）

func migrateMoment(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, user_id, content, status, comment_status,
		       read_count, IS_TOP, date_create, date_modifed
		FROM say ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id            uint
			userID        sql.NullInt64
			content       sql.NullString
			status        sql.NullString
			commentStatus sql.NullString
			readCount     sql.NullInt32
			isTop         sql.NullInt32
			dateCreate    sql.NullTime
			dateModifed   sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &content, &status, &commentStatus,
			&readCount, &isTop, &dateCreate, &dateModifed); err != nil {
			return err
		}

		m := model.Moment{
			Base:          model.Base{ID: id},
			UserID:        uint(userID.Int64),
			Content:       content.String,
			Status:        statusVarcharToUint8(status),
			CommentStatus: statusVarcharToUint8(commentStatus),
			ReadCount:     uint(readCount.Int32),
			IsTop:         isTop.Valid && isTop.Int32 == 1,
		}
		if dateCreate.Valid {
			m.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			m.UpdatedAt = dateModifed.Time
		}
		if err := dst.Create(&m).Error; err != nil {
			return fmt.Errorf("insert moment id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 13: link → friend_link
// ─────────────────────────────────────────────
// 新增 status 字段：is_use='01' → status=1（显示），'00' → status=0（隐藏）+ 软删除
// avatar_img_url → avatar_url

func migrateFriendLink(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, name, description, email, phone, site,
		       avatar_img_url, seq, date_create, date_modifed, is_use
		FROM link ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			name        sql.NullString
			description sql.NullString
			email       sql.NullString
			phone       sql.NullString
			site        sql.NullString
			avatarUrl   sql.NullString
			seq         sql.NullInt32
			dateCreate  sql.NullTime
			dateModifed sql.NullTime
			isUse       sql.NullString
		)
		if err := rows.Scan(&id, &name, &description, &email, &phone, &site,
			&avatarUrl, &seq, &dateCreate, &dateModifed, &isUse); err != nil {
			return err
		}

		fl := model.FriendLink{
			Base:        model.Base{ID: id},
			Name:        name.String,
			Description: nullStr(description),
			Email:       nullStr(email),
			Phone:       nullStr(phone),
			Site:        site.String,
			AvatarUrl:   nullStr(avatarUrl),
			Seq:         uint(seq.Int32),
			Status:      statusVarcharToUint8(isUse),
		}
		if dateCreate.Valid {
			fl.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			fl.UpdatedAt = dateModifed.Time
		}
		if isUse.Valid && isUse.String == "00" {
			t := time.Now()
			fl.DeletedAt = gorm.DeletedAt{Time: t, Valid: true}
		}
		if err := dst.Create(&fl).Error; err != nil {
			return fmt.Errorf("insert friend_link id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 14: media → media
// ─────────────────────────────────────────────
// user_id → uploader_id（字段语义改名，更明确）
// status varchar '01'/'00' → tinyint 1/0
// url 从 longtext 缩至 varchar(1000)
//
// owner_type 重映射（旧系统与新系统语义不同）：
//   旧 1 (say/碎语) → 新 2 (说说)
//   其他值保持原样

func migrateMedia(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, user_id, owner_id, owner_type, type, file_type,
		       name, url, size, status, seq, read_count, date_create, date_modifed
		FROM media ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			userID      sql.NullInt64
			ownerID     int64
			ownerType   int
			typ         int
			fileType    sql.NullString
			name        sql.NullString
			url         sql.NullString
			size        sql.NullInt64
			status      sql.NullString
			seq         sql.NullInt32
			readCount   sql.NullInt32
			dateCreate  sql.NullTime
			dateModifed sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &ownerID, &ownerType, &typ, &fileType,
			&name, &url, &size, &status, &seq, &readCount,
			&dateCreate, &dateModifed); err != nil {
			return err
		}

		urlStr := ""
		if url.Valid {
			urlStr = url.String
			// 截断超长 URL（longtext → varchar(1000)）
			if len(urlStr) > 1000 {
				urlStr = urlStr[:1000]
			}
		}

		// 旧系统用 1 表示 say（碎语），新系统用 2 表示说说；迁移时重映射。
		newOwnerType := remapMediaOwnerType(ownerType)

		m := model.Media{
			Base:       model.Base{ID: id},
			UploaderID: uint(userID.Int64),
			OwnerID:    uint(ownerID),
			OwnerType:  newOwnerType,
			Type:       uint8(typ),
			FileType:   fileType.String,
			Name:       name.String,
			URL:        urlStr,
			Size:       uint(size.Int64),
			Status:     statusVarcharToUint8(status),
			Seq:        uint(seq.Int32),
			ReadCount:  uint(readCount.Int32),
		}
		if dateCreate.Valid {
			m.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			m.UpdatedAt = dateModifed.Time
		}
		if err := dst.Create(&m).Error; err != nil {
			return fmt.Errorf("insert media id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 15: category_post → article_category
// ─────────────────────────────────────────────

func migrateArticleCategory(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query("SELECT ID, category_id, post_id FROM category_post ORDER BY ID")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, categoryID, postID uint
		if err := rows.Scan(&id, &categoryID, &postID); err != nil {
			return err
		}
		ac := model.ArticleCategory{ID: id, ArticleID: postID, CategoryID: categoryID}
		if err := dst.Create(&ac).Error; err != nil {
			return fmt.Errorf("insert article_category id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 16: tag_post → article_tag
// ─────────────────────────────────────────────

func migrateArticleTag(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query("SELECT ID, tag_id, post_id FROM tag_post ORDER BY ID")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, tagID, postID uint
		if err := rows.Scan(&id, &tagID, &postID); err != nil {
			return err
		}
		at := model.ArticleTag{ID: id, ArticleID: postID, TagID: tagID}
		if err := dst.Create(&at).Error; err != nil {
			return fmt.Errorf("insert article_tag id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 17: post_music → article_music
// ─────────────────────────────────────────────

func migrateArticleMusic(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query("SELECT ID, post_id, music_id FROM post_music ORDER BY ID")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, postID, musicID uint
		if err := rows.Scan(&id, &postID, &musicID); err != nil {
			return err
		}
		am := model.ArticleMusic{ID: id, ArticleID: postID, MusicID: musicID}
		if err := dst.Create(&am).Error; err != nil {
			return fmt.Errorf("insert article_music id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 18: recommend_post → article_recommend
// ─────────────────────────────────────────────

func migrateArticleRecommend(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query("SELECT ID, post_id, seq, date_create FROM recommend_post ORDER BY ID")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id         uint
			postID     uint
			seq        sql.NullInt32
			dateCreate sql.NullTime
		)
		if err := rows.Scan(&id, &postID, &seq, &dateCreate); err != nil {
			return err
		}
		ar := model.ArticleRecommend{
			Base:      model.Base{ID: id},
			ArticleID: postID,
			Seq:       uint(seq.Int32),
		}
		if dateCreate.Valid {
			ar.CreatedAt = dateCreate.Time
		}
		if err := dst.Create(&ar).Error; err != nil {
			return fmt.Errorf("insert article_recommend id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 19: comment → article_comment / moment_comment / guestbook
// ─────────────────────────────────────────────
// 源库 comment.type 实际值：
//   'post'      → article_comment（owner_id = article_id）
//   'say'       → moment_comment（owner_id = moment_id）
//   'guestBook' → guestbook（owner_id = 被留言的用户 id）
//
// like_num 字段丢弃（点赞数由 user_like 表汇总计算）
// is_use='00' → 软删除

func migrateComments(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, type, owner_id, from_id, content, date_create, date_modifed, is_use
		FROM comment ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			ctype       string
			ownerID     uint
			fromID      uint
			content     sql.NullString
			dateCreate  sql.NullTime
			dateModifed sql.NullTime
			isUse       sql.NullString
		)
		if err := rows.Scan(&id, &ctype, &ownerID, &fromID, &content,
			&dateCreate, &dateModifed, &isUse); err != nil {
			return err
		}

		deletedAt := gorm.DeletedAt{}
		if isUse.Valid && isUse.String == "00" {
			deletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
		}

		base := model.Base{ID: id}
		if dateCreate.Valid {
			base.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			base.UpdatedAt = dateModifed.Time
		}
		base.DeletedAt = deletedAt

		switch ctype {
		case "post":
			ac := model.ArticleComment{
				Base:      base,
				ArticleID: ownerID,
				UserID:    fromID,
				Content:   content.String,
			}
			if err := dst.Create(&ac).Error; err != nil {
				return fmt.Errorf("insert article_comment id=%d: %w", id, err)
			}
		case "say":
			mc := model.MomentComment{
				Base:     base,
				MomentID: ownerID,
				UserID:   fromID,
				Content:  content.String,
			}
			if err := dst.Create(&mc).Error; err != nil {
				return fmt.Errorf("insert moment_comment id=%d: %w", id, err)
			}
		case "guestBook":
			gb := model.Guestbook{
				Base:        base,
				OwnerUserID: ownerID,
				FromUserID:  fromID,
				Content:     content.String,
			}
			if err := dst.Create(&gb).Error; err != nil {
				return fmt.Errorf("insert guestbook id=%d: %w", id, err)
			}
		default:
			log.Printf("  警告: 未知 comment.type=%q，跳过 id=%d", ctype, id)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 20: comment_reply → article_comment_reply / moment_comment_reply / guestbook_reply
// ─────────────────────────────────────────────
// 关键变更：
//   last_id → parent_reply_id（字段改名，语义不变）
//   to_id → to_user_id，from_id → from_user_id（命名统一）
//   like_num 丢弃

func migrateCommentReply(src *sql.DB, dst *gorm.DB) error {
	typeMap, err := legacyCommentTypeMap(src)
	if err != nil {
		return err
	}

	rows, err := src.Query(`
		SELECT ID, comment_id, to_id, from_id, last_id, content,
		       date_create, date_modifed, is_use
		FROM comment_reply ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          uint
			commentID   uint
			toID        uint
			fromID      uint
			lastID      sql.NullInt64
			content     sql.NullString
			dateCreate  sql.NullTime
			dateModifed sql.NullTime
			isUse       sql.NullString
		)
		if err := rows.Scan(&id, &commentID, &toID, &fromID, &lastID, &content,
			&dateCreate, &dateModifed, &isUse); err != nil {
			return err
		}

		commentType, ok := typeMap[commentID]
		if !ok {
			// 源 comment 已被物理删除，默认归为文章评论回复
			log.Printf("  警告: comment_reply id=%d 的 comment_id=%d 不存在，默认归入 article_comment_reply", id, commentID)
			commentType = 1
		}

		parentReplyID := uint(0)
		if lastID.Valid && lastID.Int64 > 0 {
			parentReplyID = uint(lastID.Int64)
		}

		base := model.Base{ID: id}
		if dateCreate.Valid {
			base.CreatedAt = dateCreate.Time
		}
		if dateModifed.Valid {
			base.UpdatedAt = dateModifed.Time
		}
		if isUse.Valid && isUse.String == "00" {
			base.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
		}

		switch commentType {
		case 1:
			row := model.ArticleCommentReply{
				Base:          base,
				CommentID:     commentID,
				ToUserID:      toID,
				FromUserID:    fromID,
				ParentReplyID: parentReplyID,
				Content:       content.String,
			}
			if err := dst.Create(&row).Error; err != nil {
				return fmt.Errorf("insert article_comment_reply id=%d: %w", id, err)
			}
		case 2:
			row := model.MomentCommentReply{
				Base:          base,
				CommentID:     commentID,
				ToUserID:      toID,
				FromUserID:    fromID,
				ParentReplyID: parentReplyID,
				Content:       content.String,
			}
			if err := dst.Create(&row).Error; err != nil {
				return fmt.Errorf("insert moment_comment_reply id=%d: %w", id, err)
			}
		case 3:
			row := model.GuestbookReply{
				Base:          base,
				CommentID:     commentID,
				ToUserID:      toID,
				FromUserID:    fromID,
				ParentReplyID: parentReplyID,
				Content:       content.String,
			}
			if err := dst.Create(&row).Error; err != nil {
				return fmt.Errorf("insert guestbook_reply id=%d: %w", id, err)
			}
		default:
			return fmt.Errorf("comment_reply id=%d 的类型 %d 不支持", id, commentType)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 21: post_like → user_like
// ─────────────────────────────────────────────
// 源库结构：post_id（目标 ID）+ user_id + type varchar + is_use
// 目标结构：user_id + target_id + type tinyint（UNIQUE）
//
// type 映射：
//   '01' → 1（文章点赞）
//   '02' → 按评论类型拆分为 2/6/5（文章评论/碎语评论/留言）
//   '03' → 按回复类型拆分为 3/7/8（文章评论回复/碎语评论回复/留言回复）
// is_use='00' → 软删除
// 去重：相同 (user_id, target_id, type) 只保留最早一条

func migrateUserLike(src *sql.DB, dst *gorm.DB) error {
	commentTypeMap, err := legacyCommentTypeMap(src)
	if err != nil {
		return err
	}
	replyTypeMap, err := legacyReplyLikeTypeMap(src, commentTypeMap)
	if err != nil {
		return err
	}

	rows, err := src.Query(`
		SELECT ID, user_id, post_id, type, is_use, date_create
		FROM post_like ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	seen := map[string]bool{} // 用于去重

	for rows.Next() {
		var (
			id         uint
			userID     uint
			postID     uint
			likeType   sql.NullString
			isUse      sql.NullString
			dateCreate sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &postID, &likeType, &isUse, &dateCreate); err != nil {
			return err
		}

		var typeVal uint8
		switch likeType.String {
		case "01":
			typeVal = 1
		case "02":
			typeVal = commentLikeTypeFromLegacy(commentTypeMap[postID])
		case "03":
			typeVal = replyTypeMap[postID]
		default:
			typeVal = 1
		}
		if typeVal == 0 {
			log.Printf("  跳过无法识别的点赞记录 id=%d type=%s target=%d", id, likeType.String, postID)
			continue
		}

		// 去重 key
		key := fmt.Sprintf("%d-%d-%d", userID, postID, typeVal)
		if seen[key] {
			log.Printf("  跳过重复点赞 user=%d target=%d type=%d (id=%d)", userID, postID, typeVal, id)
			continue
		}
		seen[key] = true

		ul := model.UserLike{
			Base:   model.Base{ID: id},
			UserID: userID,
			// 先保留源库里的旧目标 ID，后续由 Step 25 defrag 按类型同步更新为压缩后的新 ID。
			TargetID: postID,
			Type:     typeVal,
		}
		if dateCreate.Valid {
			ul.CreatedAt = dateCreate.Time
		}
		if isUse.Valid && isUse.String == "00" {
			ul.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
		}
		if err := dst.Create(&ul).Error; err != nil {
			return fmt.Errorf("insert user_like id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

func legacyCommentTypeMap(src *sql.DB) (map[uint]uint8, error) {
	typeMap := map[uint]uint8{}
	rows, err := src.Query("SELECT ID, type FROM comment")
	if err != nil {
		return nil, fmt.Errorf("构建 comment 类型映射失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id    uint
			ctype string
		)
		if err := rows.Scan(&id, &ctype); err != nil {
			return nil, err
		}
		switch ctype {
		case "post":
			typeMap[id] = 1
		case "say":
			typeMap[id] = 2
		case "guestBook":
			typeMap[id] = 3
		}
	}
	return typeMap, rows.Err()
}

func legacyReplyLikeTypeMap(src *sql.DB, commentTypeMap map[uint]uint8) (map[uint]uint8, error) {
	replyTypeMap := map[uint]uint8{}
	rows, err := src.Query("SELECT ID, comment_id FROM comment_reply")
	if err != nil {
		return nil, fmt.Errorf("构建 comment_reply 类型映射失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			replyID   uint
			commentID uint
		)
		if err := rows.Scan(&replyID, &commentID); err != nil {
			return nil, err
		}
		replyTypeMap[replyID] = replyLikeTypeFromLegacy(commentTypeMap[commentID])
	}
	return replyTypeMap, rows.Err()
}

func commentLikeTypeFromLegacy(commentType uint8) uint8 {
	switch commentType {
	case 1:
		return 2
	case 2:
		return 6
	case 3:
		return 5
	default:
		return 0
	}
}

func replyLikeTypeFromLegacy(commentType uint8) uint8 {
	switch commentType {
	case 1:
		return 3
	case 2:
		return 7
	case 3:
		return 8
	default:
		return 0
	}
}

func dropLegacyTables(dst *gorm.DB) error {
	return dst.Exec("DROP TABLE IF EXISTS `comment_reply`").Error
}

// ─────────────────────────────────────────────
// Step 22: message → message
// ─────────────────────────────────────────────
// 丢弃的字段：from_role（角色信息通过 user_role 表查询，不在 message 里冗余存储）
// from_id → from_user_id
// post_id → article_id

func migrateMessage(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, title, content, type, type_id, from_id, post_id, comment_id, date_create
		FROM message ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id         uint
			title      sql.NullString
			content    sql.NullString
			mtype      sql.NullString
			typeID     uint
			fromID     uint
			postID     sql.NullInt64
			commentID  sql.NullInt64
			dateCreate sql.NullTime
		)
		if err := rows.Scan(&id, &title, &content, &mtype, &typeID, &fromID,
			&postID, &commentID, &dateCreate); err != nil {
			return err
		}

		m := model.Message{
			Base:       model.Base{ID: id},
			Title:      nullStr(title),
			Content:    nullStr(content),
			Type:       mtype.String,
			TypeID:     typeID,
			FromUserID: fromID,
			ArticleID:  nullUint(postID),
			CommentID:  nullUint(commentID),
		}
		if dateCreate.Valid {
			m.CreatedAt = dateCreate.Time
		}
		if err := dst.Create(&m).Error; err != nil {
			return fmt.Errorf("insert message id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 23: user_messages → user_message
// ─────────────────────────────────────────────
// 注意：源库存在大量攻击残留的孤儿记录（message_id 对应的 message 不存在），
// 这些孤儿记录在 Step 24 cleanOrphans 中统一删除。
// read_status varchar '01'=已读，'00'=未读 → is_read bool

func migrateUserMessage(src *sql.DB, dst *gorm.DB) error {
	rows, err := src.Query(`
		SELECT ID, user_id, message_id, read_status, date_create
		FROM user_messages ORDER BY ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id         uint
			userID     uint
			messageID  uint
			readStatus sql.NullString
			dateCreate sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &messageID, &readStatus, &dateCreate); err != nil {
			return err
		}

		um := model.UserMessage{
			Base:      model.Base{ID: id},
			UserID:    userID,
			MessageID: messageID,
			IsRead:    readStatus.Valid && readStatus.String == "01",
		}
		if dateCreate.Valid {
			um.CreatedAt = dateCreate.Time
		}
		if err := dst.Create(&um).Error; err != nil {
			return fmt.Errorf("insert user_message id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

// ─────────────────────────────────────────────
// Step 24: 完整性清理（孤儿记录）
// ─────────────────────────────────────────────
// 源库中有两类垃圾数据：
//   1. 攻击残留：大量 user_message 记录的 message_id 指向不存在的 message（攻击者伪造消息通知）
//   2. 历史软删除产生的悬空关联：文章/用户被删除后，对应的关联表记录未同步清理
//
// 此步骤在所有数据迁移完成后执行，清理目标库中所有无父记录的孤儿行。
// 每次迁移都会执行（不受幂等跳过逻辑影响），确保目标库数据干净。

func cleanOrphans(dst *gorm.DB) error {
	type cleanup struct {
		desc string
		sql  string
	}

	cleanups := []cleanup{
		// 攻击残留：大量 user_message.message_id 指向不存在的 message
		{
			"user_message 孤儿（message_id 无对应 message）",
			"DELETE FROM user_message WHERE message_id NOT IN (SELECT id FROM message)",
		},
		// 文章被删除后，点赞记录中仍有对该文章的引用
		{
			"user_like 孤儿（文章点赞，article 已删除）",
			"DELETE FROM user_like WHERE type=1 AND target_id NOT IN (SELECT id FROM article)",
		},
		// 文章被删除后，标签关联记录未清理
		{
			"article_tag 孤儿（article 已删除）",
			"DELETE FROM article_tag WHERE article_id NOT IN (SELECT id FROM article)",
		},
		// 文章被删除后，分类关联记录未清理
		{
			"article_category 孤儿（article 已删除）",
			"DELETE FROM article_category WHERE article_id NOT IN (SELECT id FROM article)",
		},
		// 用户被删除后，角色分配记录未清理
		{
			"user_role 孤儿（user 已删除）",
			"DELETE FROM user_role WHERE user_id NOT IN (SELECT id FROM user)",
		},
		// 用户被删除后，第三方绑定记录未清理
		{
			"social_user_auth 孤儿（user 已删除）",
			"DELETE FROM social_user_auth WHERE user_id NOT IN (SELECT id FROM user)",
		},
	}

	totalDeleted := int64(0)
	for _, c := range cleanups {
		result := dst.Exec(c.sql)
		if result.Error != nil {
			return fmt.Errorf("清理 %s 失败: %w", c.desc, result.Error)
		}
		if result.RowsAffected > 0 {
			log.Printf("  清理 %s: %d 条", c.desc, result.RowsAffected)
			totalDeleted += result.RowsAffected
		}
	}

	if totalDeleted == 0 {
		log.Printf("  无孤儿记录，数据完整")
	} else {
		log.Printf("  共清理孤儿记录 %d 条", totalDeleted)
	}
	return nil
}

// ─────────────────────────────────────────────
// Step 25: ID 整理（压缩间隙 + 重置 AUTO_INCREMENT）
// ─────────────────────────────────────────────
// 旧系统曾遭受攻击，攻击者批量写入评论和消息通知，后来被清除，导致各表 ID 出现巨大空洞。
// 例如 moment_comment 有效数据 27 条，但 MAX(id) = 166856；user_message 302 条，MAX=167426。
// 若不处理，新记录会从 166857 / 167427 开始，与实际数据量完全不匹配。
//
// 处理策略：
//   1. 按原 id 升序逐行将 id 改为 1,2,3,...（rank）
//      由于 rank(n) <= sorted_id(n) 且按升序处理，不会出现主键冲突
//   2. 同步更新所有引用该表 id 的外键列
//   3. 清理 user_like 中仍然悬空的评论点赞记录
//   4. 将 AUTO_INCREMENT 重置为 count + 1
//
// 处理顺序：先处理被引用的父表，再处理子表
//   article_comment → 被 article_comment_reply 和 user_like(type=2) 引用
//   moment_comment  → 被 moment_comment_reply 和 user_like(type=6) 引用
//   guestbook       → 被 guestbook_reply 和 user_like(type=5) 引用
//   article_comment_reply → 被 user_like(type=3) 引用
//   moment_comment_reply  → 被 user_like(type=7) 引用
//   guestbook_reply       → 被 user_like(type=8) 引用
//   user_message    → 无外键引用，独立处理

func defragIDs(dst *gorm.DB) error {
	db, err := dst.DB()
	if err != nil {
		return err
	}

	// article_comment → 更新 article_comment_reply + user_like(type=2)
	if err := defragTable(db, "article_comment", []fkRef{
		{"article_comment_reply", "comment_id", ""},
		{"user_like", "target_id", "type=2"},
	}); err != nil {
		return fmt.Errorf("defrag article_comment: %w", err)
	}

	// moment_comment → 更新 moment_comment_reply + user_like(type=6)
	if err := defragTable(db, "moment_comment", []fkRef{
		{"moment_comment_reply", "comment_id", ""},
		{"user_like", "target_id", "type=6"},
	}); err != nil {
		return fmt.Errorf("defrag moment_comment: %w", err)
	}

	// guestbook → 更新 guestbook_reply + user_like(type=5)
	if err := defragTable(db, "guestbook", []fkRef{
		{"guestbook_reply", "comment_id", ""},
		{"user_like", "target_id", "type=5"},
	}); err != nil {
		return fmt.Errorf("defrag guestbook: %w", err)
	}

	// article_comment_reply → 更新 user_like(type=3)
	if err := defragTable(db, "article_comment_reply", []fkRef{
		{"user_like", "target_id", "type=3"},
	}); err != nil {
		return fmt.Errorf("defrag article_comment_reply: %w", err)
	}

	// moment_comment_reply → 更新 user_like(type=7)
	if err := defragTable(db, "moment_comment_reply", []fkRef{
		{"user_like", "target_id", "type=7"},
	}); err != nil {
		return fmt.Errorf("defrag moment_comment_reply: %w", err)
	}

	// guestbook_reply → 更新 user_like(type=8)
	if err := defragTable(db, "guestbook_reply", []fkRef{
		{"user_like", "target_id", "type=8"},
	}); err != nil {
		return fmt.Errorf("defrag guestbook_reply: %w", err)
	}

	// user_message → 无外键引用，直接压缩
	if err := defragTable(db, "user_message", nil); err != nil {
		return fmt.Errorf("defrag user_message: %w", err)
	}

	// 清理 user_like 中仍然悬空的顶层评论点赞（评论已被删除，迁移后无对应记录）
	res, err := db.Exec(`DELETE FROM user_like WHERE type IN (2,5,6)
		AND target_id NOT IN (SELECT id FROM article_comment)
		AND target_id NOT IN (SELECT id FROM moment_comment)
		AND target_id NOT IN (SELECT id FROM guestbook)`)
	if err != nil {
		return fmt.Errorf("清理悬空评论点赞: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("  清理悬空顶层评论点赞: %d 条", n)
	}

	// 清理 user_like 中悬空的回复点赞。
	res, err = db.Exec(`DELETE FROM user_like WHERE type IN (3,7,8)
		AND target_id NOT IN (SELECT id FROM article_comment_reply)
		AND target_id NOT IN (SELECT id FROM moment_comment_reply)
		AND target_id NOT IN (SELECT id FROM guestbook_reply)`)
	if err != nil {
		return fmt.Errorf("清理悬空回复点赞: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("  清理悬空回复点赞: %d 条", n)
	}

	return nil
}

// fkRef 描述一个需要同步更新的外键引用
type fkRef struct {
	table string // 引用方表名
	col   string // 引用方外键列名
	where string // 额外过滤条件（如 "comment_type=1"）
}

// defragTable 压缩目标表的 ID 间隙，同步更新所有外键引用，最后重置 AUTO_INCREMENT。
//
// 算法：按 id 升序逐行将旧 id 替换为 rank（1,2,3,...）。
// 由于 rank(n) <= sorted_id(n)（数学可证），升序处理不会产生主键冲突。
func defragTable(db *sql.DB, table string, refs []fkRef) error {
	// 查询所有 id（升序）
	rows, err := db.Query(fmt.Sprintf("SELECT id FROM `%s` ORDER BY id ASC", table))
	if err != nil {
		return err
	}
	type idPair struct{ oldID, newID int64 }
	var pairs []idPair
	newID := int64(0)
	for rows.Next() {
		var oldID int64
		if err := rows.Scan(&oldID); err != nil {
			rows.Close()
			return err
		}
		newID++
		if oldID != newID { // 跳过 id 本就连续的行
			pairs = append(pairs, idPair{oldID, newID})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// 无间隙则只重置 AUTO_INCREMENT
	if len(pairs) == 0 {
		return resetAutoIncrement(db, table, newID+1)
	}

	log.Printf("  %s: 压缩 %d 条记录的 ID", table, len(pairs))

	// 按升序处理，将旧 id 更新为新 id（rank(n) <= old_id(n) 保证无冲突）
	for _, p := range pairs {
		_, err := db.Exec(fmt.Sprintf("UPDATE `%s` SET id=? WHERE id=?", table), p.newID, p.oldID)
		if err != nil {
			return fmt.Errorf("UPDATE %s id %d→%d: %w", table, p.oldID, p.newID, err)
		}
		// 同步更新所有引用此 id 的外键列
		for _, ref := range refs {
			query := fmt.Sprintf(
				"UPDATE `%s` SET `%s`=? WHERE `%s`=? AND %s",
				ref.table, ref.col, ref.col, ref.where,
			)
			if _, err := db.Exec(query, p.newID, p.oldID); err != nil {
				return fmt.Errorf("UPDATE %s.%s %d→%d: %w", ref.table, ref.col, p.oldID, p.newID, err)
			}
		}
	}

	return resetAutoIncrement(db, table, newID+1)
}

// resetAutoIncrement 将表的 AUTO_INCREMENT 重置为指定值（通常为 count+1）
func resetAutoIncrement(db *sql.DB, table string, next int64) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE `%s` AUTO_INCREMENT = %d", table, next))
	return err
}
