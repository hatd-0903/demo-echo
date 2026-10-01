package repository

import (
	"demo_echo/internal/models"

	"gorm.io/gorm"
)

type TagRepository interface {
	FindAll() ([]models.Tag, error)
	FindByID(id uint) (*models.Tag, error)
	FindByIDs(ids []uint) ([]models.Tag, error)
	Create(tag *models.Tag) error
	Update(tag *models.Tag) error
	Delete(id uint) error
}

type tagRepository struct {
	db *gorm.DB
}

func NewTagRepository(db *gorm.DB) TagRepository {
	return &tagRepository{db: db}
}

func (r *tagRepository) FindAll() ([]models.Tag, error) {
	var tags []models.Tag
	err := r.db.Order("name ASC").Find(&tags).Error
	return tags, err
}

func (r *tagRepository) FindByID(id uint) (*models.Tag, error) {
	var tag models.Tag
	if err := r.db.First(&tag, id).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

func (r *tagRepository) FindByIDs(ids []uint) ([]models.Tag, error) {
	tags := []models.Tag{}
	if len(ids) == 0 {
		return tags, nil
	}
	err := r.db.Where("id IN ?", ids).Find(&tags).Error
	return tags, err
}

func (r *tagRepository) Create(tag *models.Tag) error {
	return r.db.Create(tag).Error
}

func (r *tagRepository) Update(tag *models.Tag) error {
	return r.db.Save(tag).Error
}

func (r *tagRepository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		tag := models.Tag{ID: id}
		if err := tx.Model(&tag).Association("Articles").Clear(); err != nil {
			return err
		}
		return tx.Delete(&tag).Error
	})
}
