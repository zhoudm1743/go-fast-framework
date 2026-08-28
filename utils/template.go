package utils

import (
	"regexp"
	"strings"
)

// TemplateUtil 简单模板工具集。
var TemplateUtil = templateUtil{}

type templateUtil struct{}

var tplRe = regexp.MustCompile(`\{\{?\s*([a-zA-Z0-9_.]+)\s*\}?\}`)

func (r templateUtil) Replace(s string, key, value string) string {
	return strings.ReplaceAll(s, "{{"+key+"}}", value)
}

func (r templateUtil) ReplaceMap(s string, data map[string]string) string {
	return tplRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := tplRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if v, ok := data[sub[1]]; ok {
			return v
		}
		return m
	})
}

func (r templateUtil) Render(s string, data map[string]string) string {
	return r.ReplaceMap(s, data)
}
