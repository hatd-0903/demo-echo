package models

// ArticleTag là bảng nối many2many article <-> tag, khai báo tường minh
// (database.Migrate gọi SetupJoinTable) để FK có ON DELETE CASCADE:
// xoá cứng article (vd. khi xoá user) hoặc tag sẽ tự dọn dòng nối.
type ArticleTag struct {
	ArticleID uint     `gorm:"primaryKey"`
	Article   *Article `gorm:"foreignKey:ArticleID;constraint:OnDelete:CASCADE"`
	TagID     uint     `gorm:"primaryKey"`
	Tag       *Tag     `gorm:"foreignKey:TagID;constraint:OnDelete:CASCADE"`
}

func (ArticleTag) TableName() string {
	return "article_tags"
}
