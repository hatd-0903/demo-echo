package models

import "time"

// Follow là bảng quan hệ tự tham chiếu user <-> user: FollowerID đang follow FollowingID.
type Follow struct {
	// BelongsTo: người đi follow.
	FollowerID uint  `gorm:"primaryKey"`
	Follower   *User `gorm:"foreignKey:FollowerID;constraint:OnDelete:CASCADE"`

	// BelongsTo: người được follow.
	FollowingID uint  `gorm:"primaryKey;index"`
	Following   *User `gorm:"foreignKey:FollowingID;constraint:OnDelete:CASCADE"`

	CreatedAt time.Time
}

func (Follow) TableName() string {
	return "user_follows"
}
