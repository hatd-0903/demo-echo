package models

import "time"

type Tag struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:100;uniqueIndex;not null"`

	// Many2Many (chiều ngược): tag <-> article qua article_tags.
	Articles []Article `gorm:"many2many:article_tags"`

	CreatedAt time.Time
	UpdatedAt time.Time
}
