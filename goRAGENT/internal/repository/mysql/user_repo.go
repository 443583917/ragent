package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
)

// userRepo UserRepository 的 GORM 实现。
type userRepo struct{ db *gorm.DB }

// NewUserRepo 创建用户 repository。
func NewUserRepo(db *gorm.DB) repository.UserRepository {
	return &userRepo{db: db}
}

// FindByID 按主键查询（对照 auth.CurrentUser：不过滤 deleted）。
func (r *userRepo) FindByID(ctx context.Context, id string) (*model.UserDO, error) {
	var do model.UserDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find user id=%s: %w", id, err)
	}
	return &do, nil
}

func (r *userRepo) FindByUsername(ctx context.Context, username string) (*model.UserDO, error) {
	var do model.UserDO
	if err := r.db.WithContext(ctx).Scopes(notDeleted).
		Where("username = ?", username).First(&do).Error; err != nil {
		return nil, fmt.Errorf("find user username=%s: %w", username, err)
	}
	return &do, nil
}

func (r *userRepo) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.UserDO{}).Scopes(notDeleted).
		Where("username = ?", username).Count(&count).Error; err != nil {
		return false, fmt.Errorf("count user username=%s: %w", username, err)
	}
	return count > 0, nil
}

// List 分页查询（对照 listUsersReal：deleted=0，ORDER BY id DESC；keyword 模糊匹配 username）。
func (r *userRepo) List(ctx context.Context, q model.PageQuery, keyword string) ([]model.UserDO, int64, error) {
	q = q.Normalize()
	tx := r.db.WithContext(ctx).Model(&model.UserDO{}).Scopes(notDeleted)
	if keyword != "" {
		tx = tx.Where("username LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	var dos []model.UserDO
	if err := tx.Order("id DESC").Offset(q.Offset()).Limit(q.Size).Find(&dos).Error; err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	return dos, total, nil
}

func (r *userRepo) Create(ctx context.Context, u *model.UserDO) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *userRepo) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.UserDO{}).
		Scopes(notDeleted).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update user id=%s: %w", id, err)
	}
	return nil
}

func (r *userRepo) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&model.UserDO{}).
		Where("id = ?", id).Update("deleted", 1).Error; err != nil {
		return fmt.Errorf("soft delete user id=%s: %w", id, err)
	}
	return nil
}
