package repository

import (
	"demo_echo/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ArticleFilter gom các điều kiện lọc, sắp xếp và phân trang cho List.
type ArticleFilter struct {
	Tag        string // tên tag
	Author     string // username tác giả
	FollowedBy uint   // != 0: chỉ lấy bài của những người mà user này follow (feed)
	SortBy     string // một key trong ArticleSortColumns
	Desc       bool
	Limit      int
	Offset     int
}

// ArticleSortColumns là whitelist cột được phép sort, tránh SQL injection qua ORDER BY.
var ArticleSortColumns = map[string]string{
	"created_at": "articles.created_at",
	"updated_at": "articles.updated_at",
	"title":      "articles.title",
}

type ArticleRepository interface {
	List(filter ArticleFilter) ([]models.Article, int64, error)
	// FindByID / FindDetailBySlug nạp kèm Author + Tags (dùng để trả response).
	FindByID(id uint) (*models.Article, error)
	FindDetailBySlug(slug string) (*models.Article, error)
	// FindBySlug chỉ lấy dòng article, không preload (kiểm tra tồn tại / quyền sở hữu).
	FindBySlug(slug string) (*models.Article, error)
	// SlugsLike trả về mọi slug bằng base hoặc bắt đầu bằng "base-" (kể cả bản đã soft-delete).
	SlugsLike(base string) ([]string, error)
	Create(article *models.Article) error
	Update(article *models.Article, tags []models.Tag) error
	Delete(id uint) error
}

type articleRepository struct {
	db *gorm.DB
}

func NewArticleRepository(db *gorm.DB) ArticleRepository {
	return &articleRepository{db: db}
}

// withRelations eager-load author + tags. Mỗi quan hệ tốn số query cố định
// (users: 1, tags: 2 qua bảng nối) dù trang có bao nhiêu article -> không N+1.
func withRelations(db *gorm.DB) *gorm.DB {
	return db.Preload("Author", selectProfileColumns).Preload("Tags")
}

// List trả về một trang article và tổng số article khớp filter (để client tính số trang).
func (r *articleRepository) List(f ArticleFilter) ([]models.Article, int64, error) {
	column, ok := ArticleSortColumns[f.SortBy]
	if !ok {
		column = ArticleSortColumns["created_at"]
	}

	articles := []models.Article{}
	err := withRelations(r.filtered(f)).
		Order(clause.OrderByColumn{Column: clause.Column{Name: column, Raw: true}, Desc: f.Desc}).
		Order("articles.id DESC"). // tie-breaker để phân trang ổn định
		Limit(f.Limit).
		Offset(f.Offset).
		Find(&articles).Error
	if err != nil {
		return nil, 0, err
	}

	// Trang chưa đầy và không rỗng => đã chạm cuối danh sách, tổng = offset + số dòng.
	// Bỏ được câu COUNT(*) trong trường hợp phổ biến nhất (ít dữ liệu / trang cuối).
	if n := len(articles); n > 0 && n < f.Limit {
		return articles, int64(f.Offset + n), nil
	}
	var total int64
	if err := r.filtered(f).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	return articles, total, nil
}

// filtered dựng query có WHERE theo filter. Dùng subquery thay vì JOIN để
// không bị nhân bản dòng (article nhiều tag) và COUNT luôn đúng.
func (r *articleRepository) filtered(f ArticleFilter) *gorm.DB {
	q := r.db.Model(&models.Article{})
	if f.Tag != "" {
		q = q.Where("articles.id IN (?)", r.db.Table("article_tags").
			Select("article_tags.article_id").
			Joins("JOIN tags ON tags.id = article_tags.tag_id").
			Where("tags.name = ?", f.Tag))
	}
	if f.Author != "" {
		q = q.Where("articles.author_id IN (?)", r.db.Model(&models.User{}).
			Select("id").
			Where("username = ?", f.Author))
	}
	if f.FollowedBy != 0 {
		q = q.Where("articles.author_id IN (?)", r.db.Model(&models.Follow{}).
			Select("following_id").
			Where("follower_id = ?", f.FollowedBy))
	}
	return q
}

func (r *articleRepository) FindByID(id uint) (*models.Article, error) {
	var article models.Article
	if err := withRelations(r.db).First(&article, id).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *articleRepository) FindDetailBySlug(slug string) (*models.Article, error) {
	var article models.Article
	if err := withRelations(r.db).Where("slug = ?", slug).First(&article).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *articleRepository) FindBySlug(slug string) (*models.Article, error) {
	var article models.Article
	if err := r.db.Where("slug = ?", slug).First(&article).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

// SlugsLike lấy tất cả slug có thể đụng độ trong 1 query, thay vì hỏi từng slug
// "base", "base-2", "base-3"... (N query khi có N bài trùng title).
// Slug chỉ gồm [a-z0-9-] nên không có ký tự đại diện % / _ cần escape.
func (r *articleRepository) SlugsLike(base string) ([]string, error) {
	var slugs []string
	err := r.db.Unscoped().Model(&models.Article{}).
		Where("slug = ? OR slug LIKE ?", base, base+"-%").
		Pluck("slug", &slugs).Error
	return slugs, err
}

// Create chỉ chèn article + dòng nối article_tags. Omit("Tags.*") ngăn GORM
// upsert lại chính các tag (INSERT INTO tags ... ON DUPLICATE KEY UPDATE) vốn đã tồn tại.
func (r *articleRepository) Create(article *models.Article) error {
	return r.db.Omit("Author", "Tags.*").Create(article).Error
}

// Update chỉ ghi các cột có thể sửa; nếu tags != nil thì đồng bộ bảng nối article_tags
// trực tiếp (xoá tag bị bỏ + chèn tag mới), thay cho Association.Replace vốn upsert
// lại bảng tags và UPDATE thêm updated_at của article.
func (r *articleRepository) Update(article *models.Article, tags []models.Tag) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(article).Select("title", "slug", "body").Updates(article).Error; err != nil {
			return err
		}
		if tags == nil {
			return nil
		}

		tagIDs := make([]uint, len(tags))
		rows := make([]map[string]any, len(tags))
		for i, t := range tags {
			tagIDs[i] = t.ID
			rows[i] = map[string]any{"article_id": article.ID, "tag_id": t.ID}
		}

		var del *gorm.DB
		if len(tagIDs) == 0 {
			del = tx.Exec("DELETE FROM article_tags WHERE article_id = ?", article.ID)
		} else {
			del = tx.Exec("DELETE FROM article_tags WHERE article_id = ? AND tag_id NOT IN ?", article.ID, tagIDs)
		}
		if err := del.Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			// Bảng nối không có model nên OnConflict{DoNothing} sinh "ON DUPLICATE KEY UPDATE" rỗng;
			// chỉ rõ no-op tag_id = tag_id để bỏ qua dòng đã tồn tại.
			skipExisting := clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{"tag_id": gorm.Expr("tag_id")})}
			if err := tx.Table("article_tags").Clauses(skipExisting).Create(rows).Error; err != nil {
				return err
			}
		}
		article.Tags = tags
		return nil
	})
}

func (r *articleRepository) Delete(id uint) error {
	return r.db.Delete(&models.Article{}, id).Error
}
