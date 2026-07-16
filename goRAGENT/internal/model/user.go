package model

// UserDO t_user 用户
type UserDO struct {
	ID       int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Username string `gorm:"column:username"`
	Password string `gorm:"column:password"`
	Role     string `gorm:"column:role"`
	Avatar   string `gorm:"column:avatar"`
}

func (UserDO) TableName() string { return "t_user" }
