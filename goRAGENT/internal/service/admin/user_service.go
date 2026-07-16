package admin

import (
	"context"

	"goRAGENT/internal/model"
	"goRAGENT/internal/repository"
	"goRAGENT/internal/service/auth"
	"goRAGENT/pkg/errs"
	"go.uber.org/zap"
)

// UserService 用户管理服务接口。
type UserService interface {
	List(ctx context.Context, q model.PageQuery) ([]model.UserVO, int64, error)
	Create(ctx context.Context, req model.UserCreateReq) error
	Update(ctx context.Context, id string, req model.UserUpdateReq) error
	Delete(ctx context.Context, id string) error
	ChangePassword(ctx context.Context, id, newPassword string) error
}

type userService struct {
	repo   repository.UserRepository
	hasher auth.PasswordHasher
}

// NewUserService 创建用户管理服务。
func NewUserService(repo repository.UserRepository, hasher auth.PasswordHasher) UserService {
	return &userService{repo: repo, hasher: hasher}
}

func (s *userService) List(ctx context.Context, q model.PageQuery) ([]model.UserVO, int64, error) {
	dos, total, err := s.repo.List(ctx, q, "")
	if err != nil {
		zap.L().Error("查询用户列表失败", zap.Error(err))
		return nil, 0, errs.WrapServer(err, "查询失败")
	}
	vos := make([]model.UserVO, 0, len(dos))
	for _, d := range dos {
		vos = append(vos, model.UserVO{
			ID: d.ID, Username: d.Username, Role: d.Role, Avatar: d.Avatar,
		})
	}
	return vos, total, nil
}

func (s *userService) Create(ctx context.Context, req model.UserCreateReq) error {
	if len(req.Username) < 2 || len(req.Password) < 4 {
		return errs.Param("账号至少2位，密码至少4位")
	}

	exists, err := s.repo.ExistsByUsername(ctx, req.Username)
	if err != nil {
		zap.L().Error("检查用户名冲突失败", zap.Error(err))
		return errs.WrapServer(err, "创建失败")
	}
	if exists {
		return errs.Business("账号已存在")
	}

	role := req.Role
	if role == "" {
		role = model.RoleUser
	}

	user := model.UserDO{
		Username: req.Username,
		Password: s.hasher.Hash(req.Password),
		Role:     role,
	}
	if err := s.repo.Create(ctx, &user); err != nil {
		zap.L().Error("创建用户失败", zap.Error(err))
		return errs.WrapBusiness(err, "创建失败")
	}
	return nil
}

func (s *userService) Update(ctx context.Context, id string, req model.UserUpdateReq) error {
	updates := map[string]any{}
	if req.Role != nil {
		updates["role"] = *req.Role
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.repo.UpdateFields(ctx, id, updates); err != nil {
		zap.L().Error("更新用户失败", zap.Error(err))
		return errs.WrapBusiness(err, "更新失败")
	}
	return nil
}

func (s *userService) Delete(ctx context.Context, id string) error {
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		zap.L().Error("删除用户失败", zap.Error(err))
		return errs.WrapBusiness(err, "删除失败")
	}
	return nil
}

func (s *userService) ChangePassword(ctx context.Context, id, newPassword string) error {
	if len(newPassword) < 4 {
		return errs.Param("密码至少4位")
	}
	pwdHash := s.hasher.Hash(newPassword)
	if err := s.repo.UpdateFields(ctx, id, map[string]any{"password": pwdHash}); err != nil {
		zap.L().Error("修改密码失败", zap.Error(err))
		return errs.WrapBusiness(err, "修改失败")
	}
	return nil
}
