package auth

import (
	"crypto/md5"
	"fmt"
)

// PasswordHasher 密码哈希抽象，屏蔽具体算法便于后续替换（如 bcrypt）。
type PasswordHasher interface {
	Hash(plain string) string
	Verify(plain, hashed string) bool
}

// MD5PasswordHasher MD5 实现（兼容存量数据，逻辑取自原 model.MD5Hash）。
type MD5PasswordHasher struct{}

// NewMD5PasswordHasher 创建 MD5 密码哈希器。
func NewMD5PasswordHasher() PasswordHasher { return MD5PasswordHasher{} }

// Hash 计算明文密码的 MD5 十六进制小写摘要。
func (MD5PasswordHasher) Hash(plain string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(plain)))
}

// Verify 校验明文密码与存量哈希是否匹配。
func (h MD5PasswordHasher) Verify(plain, hashed string) bool {
	return h.Hash(plain) == hashed
}
