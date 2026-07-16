package model

import (
	"crypto/md5"
	"fmt"
)

// UserDO t_user 用户
type UserDO struct {
	ID       int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Username string `gorm:"column:username"`
	Password string `gorm:"column:password"`
	Role     string `gorm:"column:role"`
	Avatar   string `gorm:"column:avatar"`
}

func (UserDO) TableName() string { return "t_user" }

// MD5Hash MD5 哈希（供多处复用）
func MD5Hash(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }
