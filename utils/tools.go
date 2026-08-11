package utils

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

var ToolsUtil = toolsUtil{}

type toolsUtil struct{}

func (r toolsUtil) Md5(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	hash := md5.New()
	hash.Write([]byte(s))
	return hex.EncodeToString(hash.Sum(nil))
}

func (r toolsUtil) Sha1(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	hash := sha1.New()
	hash.Write([]byte(s))
	return hex.EncodeToString(hash.Sum(nil))
}

// CompareSha1 compares two SHA1 hashes
func (r toolsUtil) CompareSha1(s1, original string) bool {
	return r.Sha1(s1) == original
}

// CompareMd5 compares two MD5 hashes
func (r toolsUtil) CompareMd5(s1, original string) bool {
	return r.Md5(s1) == original
}

// GenerateSalt 生成随机盐（32位十六进制字符串）
func (r toolsUtil) GenerateSalt() string {
	salt := make([]byte, 16)
	rand.Read(salt)
	return hex.EncodeToString(salt)
}

// HashPassword 使用盐加密密码
func (r toolsUtil) HashPassword(password, salt string) string {
	return r.Md5(password + salt)
}

// VerifyPassword 验证密码
func (r toolsUtil) VerifyPassword(password, salt, hashedPassword string) bool {
	return r.HashPassword(password, salt) == hashedPassword
}
