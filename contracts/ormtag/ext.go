package ormtag

import (
	"fmt"
	"strconv"
	"strings"
)

// ext tag 解析（gorm 风格 key:value; 分号分隔，文档 4.3）。
//
// 键全集（键名匹配大小写不敏感）：
//   - check:<expr>                 → Check（表达式原文，可含空格/逗号/比较符）
//   - index:<完整选项>             → IndexOptions（原文存储，覆盖 orm index token 同维度）
//   - autoIncrementIncrement:N     → AutoIncrementIncrement（int64，非法值报错）
//   - migration:false              → IgnoreMigration=true（读写但不参与建表）
//   - timePrecision:milli|nano     → TimePrecision（原文存储，消费方校验 milli/nano）
//   - perm:create|update           → PermCreateOnly / PermUpdateOnly（逗号分隔可同时声明）
//
// 未知 ext key 不报错（留给驱动侧 Warn 日志，文档 11.16）。

// parseExtTag 解析 ext tag，返回 ExtMeta 供 ModelMeta.Exts 收录。
func parseExtTag(fieldName, tagStr string) (ExtMeta, error) {
	var meta ExtMeta
	for _, sec := range splitGormSections(tagStr) {
		switch strings.ToLower(sec.key) {
		case "check":
			meta.Check = sec.value
		case "index":
			meta.IndexOptions = sec.value
		case "autoincrementincrement":
			n, err := strconv.ParseInt(sec.value, 10, 64)
			if err != nil {
				return meta, fmt.Errorf("ormtag: 字段 %q 的 ext tag %q: autoIncrementIncrement 需要 int64 值，得到 %q",
					fieldName, tagStr, sec.value)
			}
			meta.AutoIncrementIncrement = n
		case "migration":
			if strings.EqualFold(strings.TrimSpace(sec.value), "false") {
				meta.IgnoreMigration = true
			}
		case "timeprecision":
			meta.TimePrecision = sec.value
		case "perm":
			for _, p := range splitFieldList(sec.value) {
				switch strings.ToLower(p) {
				case "create":
					meta.PermCreateOnly = true
				case "update":
					meta.PermUpdateOnly = true
				}
			}
		default:
			// 未知 ext key：不报错，记录原文供驱动侧 Warn（文档 11.16）
			meta.UnknownKeys = append(meta.UnknownKeys, sec.key)
		}
	}
	return meta, nil
}
