package repository

import (
	"errors"

	"demo_echo/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FollowRepository interface {
	Follow(followerID, followingID uint) error
	Unfollow(followerID, followingID uint) error
	IsFollowing(followerID, followingID uint) (bool, error)
}

type followRepository struct {
	db *gorm.DB
}

func NewFollowRepository(db *gorm.DB) FollowRepository {
	return &followRepository{db: db}
}

// Follow là idempotent: follow lại người đã follow không báo lỗi.
func (r *followRepository) Follow(followerID, followingID uint) error {
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.Follow{FollowerID: followerID, FollowingID: followingID}).Error
}

func (r *followRepository) Unfollow(followerID, followingID uint) error {
	return r.db.Where("follower_id = ? AND following_id = ?", followerID, followingID).
		Delete(&models.Follow{}).Error
}

func (r *followRepository) IsFollowing(followerID, followingID uint) (bool, error) {
	var f models.Follow
	err := r.db.Where("follower_id = ? AND following_id = ?", followerID, followingID).
		Take(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}
