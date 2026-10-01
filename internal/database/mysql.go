package database

import (
	"fmt"
	"log"
	"time"

	"demo_echo/internal/config"
	"demo_echo/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect mở kết nối MySQL qua GORM và cấu hình connection pool.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Info // in mọi câu SQL khi dev
	if cfg.IsProduction() {
		logLevel = logger.Warn // chỉ in query chậm / lỗi
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger:         logger.Default.LogMode(logLevel),
		TranslateError: true, // chuyển lỗi MySQL (vd. duplicate key) thành gorm.ErrDuplicatedKey
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("connected to MySQL")
	return db, nil
}

// Migrate tạo/cập nhật bảng theo các model rồi chạy các bước sửa schema mà
// AutoMigrate không tự làm (AutoMigrate chỉ thêm, không sửa/xoá).
func Migrate(db *gorm.DB) error {
	// Bảng nối article_tags dùng model tường minh để có FK ON DELETE CASCADE.
	if err := db.SetupJoinTable(&models.Article{}, "Tags", &models.ArticleTag{}); err != nil {
		return err
	}
	if err := db.SetupJoinTable(&models.Tag{}, "Articles", &models.ArticleTag{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Tag{},
		&models.Article{},
		&models.ArticleTag{},
		&models.Comment{},
		&models.Follow{},
	); err != nil {
		return err
	}
	if err := ensureCascadeConstraints(db); err != nil {
		return err
	}
	return dropUserSoftDelete(db)
}

// cascadeRelations: mọi FK phải là ON DELETE CASCADE để xoá cứng user dọn sạch dữ liệu liên quan.
var cascadeRelations = []struct {
	model any
	field string
}{
	{&models.User{}, "Articles"},
	{&models.User{}, "Comments"},
	{&models.Article{}, "Comments"},
	{&models.ArticleTag{}, "Article"},
	{&models.ArticleTag{}, "Tag"},
	{&models.Follow{}, "Follower"},
	{&models.Follow{}, "Following"},
}

// ensureCascadeConstraints tạo lại FK nào chưa phải CASCADE. Cần cho DB tạo bởi phiên bản
// cũ: khi đó tag constraint nằm ở phía BelongsTo nên GORM bỏ qua và tạo FK "NO ACTION",
// và AutoMigrate không bao giờ sửa FK đã tồn tại.
func ensureCascadeConstraints(db *gorm.DB) error {
	for _, rel := range cascadeRelations {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(rel.model); err != nil {
			return err
		}
		r, ok := stmt.Schema.Relationships.Relations[rel.field]
		if !ok {
			return fmt.Errorf("relation %s.%s not found", stmt.Schema.Name, rel.field)
		}
		c := r.ParseConstraint()
		if c == nil {
			return fmt.Errorf("relation %s.%s has no constraint", stmt.Schema.Name, rel.field)
		}

		var rule string
		db.Raw(`SELECT delete_rule FROM information_schema.referential_constraints
			WHERE constraint_schema = DATABASE() AND table_name = ? AND constraint_name = ?`,
			c.Schema.Table, c.Name).Scan(&rule)
		if rule == "CASCADE" {
			continue
		}
		if rule != "" {
			log.Printf("migrate: recreating %s.%s as ON DELETE CASCADE (was %s)", c.Schema.Table, c.Name, rule)
			if err := db.Migrator().DropConstraint(rel.model, rel.field); err != nil {
				return err
			}
		}
		if err := db.Migrator().CreateConstraint(rel.model, rel.field); err != nil {
			return err
		}
	}
	return nil
}

// dropUserSoftDelete: bản cũ dùng soft delete cho user. Xoá hẳn các user đã bị
// "xoá mềm" (FK CASCADE dọn dữ liệu của họ) rồi bỏ cột deleted_at.
func dropUserSoftDelete(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&models.User{}, "deleted_at") {
		return nil
	}
	res := db.Exec("DELETE FROM users WHERE deleted_at IS NOT NULL")
	if res.Error != nil {
		return res.Error
	}
	log.Printf("migrate: purged %d soft-deleted users, dropping users.deleted_at", res.RowsAffected)
	return db.Migrator().DropColumn(&models.User{}, "deleted_at")
}
