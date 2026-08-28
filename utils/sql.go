package utils

import (
	"strings"
)

// SqlUtil SQL 安全工具集。
var SqlUtil = sqlUtil{}

type sqlUtil struct{}

func (r sqlUtil) EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func (r sqlUtil) SafeOrder(field string, whitelist []string, dir string) (string, error) {
	ok := false
	for _, w := range whitelist {
		if field == w {
			ok = true
			break
		}
	}
	if !ok {
		return "", ErrInvalidOrderField
	}
	dir = strings.ToUpper(strings.TrimSpace(dir))
	if dir != "ASC" && dir != "DESC" {
		dir = "ASC"
	}
	return field + " " + dir, nil
}

// ErrInvalidOrderField 排序字段不在白名单。
var ErrInvalidOrderField = errInvalidOrderField{}

type errInvalidOrderField struct{}

func (errInvalidOrderField) Error() string { return "invalid order field" }
