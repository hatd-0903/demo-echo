package repository

import "gorm.io/gorm"

// selectProfileColumns giới hạn cột khi Preload user làm author: chỉ lấy những gì
// ProfileResponse cần, không kéo password hash / email về bộ nhớ.
func selectProfileColumns(db *gorm.DB) *gorm.DB {
	return db.Select("id", "username", "bio", "image")
}
