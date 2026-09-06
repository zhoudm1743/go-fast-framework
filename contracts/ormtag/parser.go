package ormtag

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// ── SQL 类型白名单 ────────────────────────────────────────────────────────

// sqlTypeWhitelist orm tag 允许的 SQL 类型名全集（大小写不敏感，键为大写）。
//
// 来源：xorm.io/xorm@v1.4.1 schemas/type.go 的 SqlTypes map（2026-09-06 逐项核对），
// 硬编码等价清单——框架代码禁止 import xorm（仅标准库约束，文档 §6.1）。
//
// 说明：
//   - xorm SqlTypes 中带 UNSIGNED 前缀的复合类型（UNSIGNED BIT/TINYINT/SMALLINT/
//     MEDIUMINT/INT/BIGINT/DECIMAL/FLOAT）含空格、无法成为单个 tag token，
//     xorm 以 `unsigned` 标志 token + 基础类型组合表达（UnsignedTagHandler），
//     故不在此收录、由 FieldMeta.Unsigned 布尔位承载；
//   - JSON/JSONB 同时注册为 xorm tag handler（JSONTagHandler/JSONBTagHandler），
//     token 分发中优先按 handler 处理（置 JSON 布尔位），不写入 Type 字段；
//   - "INT8" 亦为 xorm v1.4.1 SqlTypes 键，一并收录。
var sqlTypeWhitelist = map[string]bool{
	"BIT": true, "TINYINT": true, "SMALLINT": true, "MEDIUMINT": true,
	"INT": true, "INTEGER": true, "BIGINT": true, "NUMBER": true, "INT8": true,

	"ENUM": true, "SET": true, "XML": true,

	"CHAR": true, "NCHAR": true, "VARCHAR": true, "VARCHAR2": true, "NVARCHAR": true,
	"TINYTEXT": true, "TEXT": true, "NTEXT": true, "MEDIUMTEXT": true, "LONGTEXT": true,
	"UUID": true, "CLOB": true, "SYSNAME": true,

	"DATE": true, "DATETIME": true, "TIME": true, "TIMESTAMP": true,
	"TIMESTAMPZ": true, "SMALLDATETIME": true, "YEAR": true,

	"DECIMAL": true, "NUMERIC": true, "REAL": true, "FLOAT": true, "DOUBLE": true,
	"MONEY": true, "SMALLMONEY": true,

	"BINARY": true, "VARBINARY": true, "TINYBLOB": true, "BLOB": true,
	"MEDIUMBLOB": true, "LONGBLOB": true, "BYTEA": true, "UNIQUEIDENTIFIER": true,

	"BOOL": true, "BOOLEAN": true,

	"SERIAL": true, "BIGSERIAL": true,

	"ARRAY": true,
}

// ── token 化状态机（对齐 xorm splitTag 规则 + 框架级非法语法报错） ──────────

// rawToken 单个 orm tag token。
type rawToken struct {
	name    string   // token 名：带引号 token 为去引号/反转义后的内容；裸 token 为原文
	quoted  bool     // 是否以单引号书写：'column_name'
	params  []string // 括号参数（已去首尾空白；引号参数保留原文，由使用方 unquoteParam）
	raw     string   // token 原文（含引号/括号，如 "varchar(100)"、"decimal(10,2)"）
	hasCond bool     // 是否携带括号参数
}

// splitTag 对齐 xorm tags/tag.go splitTag 的切分规则（文档 2.2 实验结论）：
//   - 按空格切分 token；单引号内的空格受保护（default 'act ive' 是一个含空格 token）；
//   - 逗号必须位于引号或括号内，否则报错（xorm 原生即报错）；
//   - 括号参数按逗号切分为 params，引号参数原文保留（如 comment('用户昵称')）。
//
// 在 xorm 基础上收紧（框架刻意更严，文档 4.1/11.2）：
//   - 未闭合引号/括号、括号嵌套、多余右括号、token 中途出现引号 → 明确报错
//     （xorm 部分场景静默吞掉）；
//   - 引号内 ” 转义为字面单引号（SQL 字符串惯例；xorm 逐字符翻转引号状态
//     无法表达转义，如 comment('it”s') 由本解析器提取为 it's）。
func splitTag(tagStr string) ([]rawToken, error) {
	tagStr = strings.TrimSpace(tagStr)
	if tagStr == "" {
		return nil, nil
	}
	var (
		tokens     []rawToken
		cur        rawToken
		curStart   = 0  // 当前 token 起始字节下标
		paramStart = 0  // 当前括号参数起始字节下标
		inQuote    bool // 位于单引号内
		inParen    bool // 位于括号内
		curQuoted  bool // 当前 token 的 name 是否为引号 token
		seenParen  bool // 当前 token 是否出现过括号
		nameEnd    = -1 // token name 结束位置（引号收尾/左括号处）
		haveName   bool // 当前 token 是否已开始 name
	)
	appendChild := func(raw string) {
		cur.params = append(cur.params, strings.TrimSpace(raw))
	}
	flush := func(end int) {
		switch {
		case curQuoted:
			cur.name = unquote(tagStr[curStart:nameEnd])
		case nameEnd >= 0:
			cur.name = tagStr[curStart:nameEnd]
		default:
			cur.name = tagStr[curStart:end]
		}
		cur.raw = tagStr[curStart:end]
		cur.quoted = curQuoted
		cur.hasCond = len(cur.params) > 0
		tokens = append(tokens, cur)
		cur = rawToken{}
		curQuoted, haveName, seenParen = false, false, false
		nameEnd = -1
	}

	for i := 0; i < len(tagStr); i++ {
		t := tagStr[i]
		switch t {
		case '\'':
			if inQuote {
				// 引号内 '' 为转义的单引号（跳过第二个引号，保持引号状态）
				if i+1 < len(tagStr) && tagStr[i+1] == '\'' {
					i++
					continue
				}
				inQuote = false
				if !inParen && curQuoted {
					nameEnd = i + 1 // 引号 token name 结束（含收尾引号，unquote 时去除）
				}
				continue
			}
			if inParen {
				inQuote = true // 括号参数内的引号：comment('用户昵称')、default('x')
				continue
			}
			if haveName {
				return nil, fmt.Errorf("偏移 %d 处引号位置非法：引号 token 必须独立成词", i)
			}
			inQuote = true
			curQuoted = true
			haveName = true
			curStart = i
		case ' ':
			if inQuote {
				continue // 引号内空格受保护：default 'act ive'
			}
			if inParen {
				paramStart = i + 1 // xorm：括号内空格推进参数起点
				continue
			}
			if haveName {
				flush(i)
			}
			curStart = i + 1
		case ',':
			if !inQuote && !inParen {
				return nil, fmt.Errorf("偏移 %d 处逗号必须位于引号或括号内", i)
			}
			if !inQuote && inParen {
				appendChild(tagStr[paramStart:i])
				paramStart = i + 1
			}
		case '(':
			if inQuote {
				continue
			}
			if inParen || seenParen {
				return nil, fmt.Errorf("偏移 %d 处括号嵌套/重复不合法", i)
			}
			inParen = true
			seenParen = true
			if haveName {
				if nameEnd < 0 {
					nameEnd = i
				}
			} else {
				haveName = true
				curStart = i
				nameEnd = i
			}
			paramStart = i + 1
		case ')':
			if inQuote {
				continue
			}
			if !inParen {
				return nil, fmt.Errorf("偏移 %d 处出现多余的右括号", i)
			}
			inParen = false
			appendChild(tagStr[paramStart:i])
		default:
			if !inQuote && !haveName {
				haveName = true
				curStart = i
			}
		}
	}
	if inQuote {
		return nil, fmt.Errorf("未闭合的单引号")
	}
	if inParen {
		return nil, fmt.Errorf("未闭合的括号")
	}
	if haveName {
		flush(len(tagStr))
	}
	return tokens, nil
}

// unquote 去除首尾单引号并反转义 ” → '（SQL 字符串惯例）。
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
	}
	return strings.ReplaceAll(s, "''", "'")
}

// unquoteParam 去除括号参数外层单引号（存在时）并反转义。
func unquoteParam(s string) string {
	return unquote(strings.TrimSpace(s))
}

// ── 单字段 orm tag 解析 ──────────────────────────────────────────────────

// parseFieldTag 解析单字段的 orm tag（xorm 语法）。返回的 FieldMeta 不含
// FieldName/FieldIndex（由调用方补充），RawTag 已填充。
// 命中白名单外裸 token / 禁用 token / 非法语法时返回错误，
// 错误信息含字段名与 tag 原文（文档 §6.2/§6.3）。
func parseFieldTag(fieldName, tagStr string) (FieldMeta, error) {
	tagStr = strings.TrimSpace(tagStr)
	meta := FieldMeta{RawTag: tagStr}
	if tagStr == "" {
		return meta, nil
	}

	tokens, err := splitTag(tagStr)
	if err != nil {
		return meta, fmt.Errorf("ormtag: 字段 %q 的 orm tag %q 语法非法: %w", fieldName, tagStr, err)
	}

	// token 白名单分发（xorm 26 handler + 全部 SQL 类型，文档 4.2）。
	// not null 双 token 组合消费；default/collate 无参形态消费下一 token
	// （对齐 xorm DefaultTagHandler/CollateTagHandler 的 ignoreNext 语义）。
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]

		// 列名强制单引号（文档 4.1）：'column_name'
		if tok.quoted {
			if tok.hasCond {
				return meta, fieldErr(fieldName, tagStr, "引号 token 不接受括号参数: "+tok.raw)
			}
			meta.ColumnName = tok.name
			continue
		}

		switch strings.ToUpper(tok.name) {
		case "-": // 忽略字段（不建列/不读/不写）
			meta.Ignore = true
		case "<-": // 只读（xorm OnlyFromDB：读回填充、写入忽略）——方向防回归见 11.2
			meta.ReadOnly = true
		case "->": // 只写（xorm OnlyToDB：写入落库、读回不填）
			meta.WriteOnly = true
		case "PK":
			meta.PrimaryKey = true
		case "AUTOINCR":
			meta.AutoIncrement = true
		case "NOTNULL":
			meta.NotNull = true
		case "NULL":
			meta.Null = true
		case "NOT": // 仅作为 `not null` 组合消费（xorm NULLTagHandler 读 preTag 语义）
			if i+1 >= len(tokens) || tokens[i+1].quoted || !strings.EqualFold(tokens[i+1].name, "null") {
				return meta, fieldErr(fieldName, tagStr, `NOT token 必须与 NULL 组合书写为 "not null"`)
			}
			meta.NotNull = true
			i++
		case "INDEX":
			meta.Index = true
			if tok.hasCond {
				meta.IndexName = unquoteParam(tok.params[0])
			}
		case "UNIQUE":
			meta.Unique = true
			if tok.hasCond {
				meta.UniqueName = unquoteParam(tok.params[0])
			}
		case "DEFAULT": // default 0 / default 'active' / default(0) 三形态（文档 4.1/11.2）
			if tok.hasCond {
				meta.Default = unquoteParam(tok.params[0])
			} else {
				if i+1 >= len(tokens) {
					return meta, fieldErr(fieldName, tagStr, "default 缺少值（支持 default 0 / default 'x' / default(x) 三种形态）")
				}
				meta.Default = tokens[i+1].name // 引号 token 已去引号，裸 token 取原文
				i++                             // 消费值 token（xorm ignoreNext 语义）
			}
			meta.HasDefault = true
		case "COMMENT":
			if tok.hasCond {
				meta.Comment = unquoteParam(tok.params[0])
			}
		case "CREATED":
			meta.Created = true
		case "UPDATED":
			meta.Updated = true
		case "DELETED": // 禁用（文档 4.4）：xorm 原生软删除与框架业务级软删除冲突
			return meta, fieldErr(fieldName, tagStr,
				`禁用 token "deleted"（xorm 原生软删除与框架业务级软删除 OnlyTrashed/Restore/ForceDelete 语义冲突）；`+
					`替代方案：移除该 token，使用框架软删除（deleted_at 列仅声明 'deleted_at' index default(0)）`)
		case "VERSION":
			meta.Version = true
		case "UTC":
			meta.UTC = true
		case "LOCAL":
			meta.Local = true
		case "EXTENDS":
			meta.Extends = true
			if tok.hasCond {
				meta.ExtendsPrefix = unquoteParam(tok.params[0])
			}
		case "UNSIGNED":
			meta.Unsigned = true
		case "COLLATE":
			if tok.hasCond {
				meta.Collate = unquoteParam(tok.params[0])
			} else if i+1 < len(tokens) {
				meta.Collate = tokens[i+1].name
				i++
			} else {
				return meta, fieldErr(fieldName, tagStr, "collate 缺少参数（支持 collate(utf8mb4_bin) 形态）")
			}
		case "JSON", "JSONB": // jsonb 对齐 xorm JSONBTagHandler：IsJSONB 与 IsJSON 同时置位
			meta.JSON = true
		case "CACHE", "NOCACHE": // 禁用（文档 4.4）：xorm 内建 LRU 缓存与框架查询缓存冲突
			return meta, fieldErr(fieldName, tagStr,
				fmt.Sprintf("禁用 token %q（xorm 内建 LRU 缓存与框架 Query().Cache() 缓存体系冲突，并存会导致失效策略失控、脏读）；"+
					"替代方案：移除该 token，改用框架查询缓存 Query().Cache()", strings.ToLower(tok.name)))
		default:
			if sqlTypeWhitelist[strings.ToUpper(tok.name)] {
				meta.Type = tok.raw // 类型 token 原文（含括号参数，如 decimal(10,2)）
				continue
			}
			// 未加引号的未知裸 token：xorm 会静默当作列名建垃圾列（文档 2.2 实测，
			// 如 orm:"serializer:json" 会建出 "serializer:json" 列）——白名单校验的
			// 根本原因，解析期直接报错（文档 4.4）。
			return meta, fieldErr(fieldName, tagStr,
				fmt.Sprintf("未加引号的未知裸 token %q（xorm 会将其静默当作列名建立垃圾列）；"+
					"列名必须用单引号书写（如 'user_name'），确属新方言 token 时先扩展 ormtag 白名单", tok.raw))
		}
	}
	return meta, nil
}

// fieldErr 构造含字段名与 tag 原文的解析错误。
func fieldErr(fieldName, tagStr, msg string) error {
	return fmt.Errorf("ormtag: 字段 %q 的 orm tag %q: %s", fieldName, tagStr, msg)
}

// ParseField 解析单个字段的 orm tag（尽力解析，不返回错误）。
// 命中禁用 token/未知裸 token/非法语法时返回已解析部分（RawTag 保留原文供定位）。
func ParseField(field reflect.StructField) FieldMeta {
	meta, _ := parseFieldTag(field.Name, field.Tag.Get("orm"))
	meta.FieldName = field.Name
	return meta
}

// ── 模型级解析与缓存 ─────────────────────────────────────────────────────

// modelCache 按 reflect.Type 缓存解析结果（文档 §6.3 第 4 点：sync.Map + 解析幂等）。
var modelCache sync.Map // reflect.Type → *ModelMeta

// Parse 解析并缓存模型元数据。model 为 struct、struct 指针（含多级指针）。
// 命中缓存返回同一 *ModelMeta 指针，调用方不得修改返回值。
// 命中禁用 token（cache/nocache/deleted）、未加引号的未知裸 token、非法语法、
// rel 复合键不等长、循环嵌入等错误时返回 error（错误信息含字段名与 tag 原文）。
func Parse(model any) (*ModelMeta, error) {
	t := indirectType(model)
	if t == nil {
		return nil, fmt.Errorf("ormtag: Parse(model) 收到 nil")
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("ormtag: Parse(model) 要求 struct 或 struct 指针，得到 %s", t.Kind())
	}
	if cached, ok := modelCache.Load(t); ok {
		return cached.(*ModelMeta), nil
	}

	meta := &ModelMeta{
		Type: t,
		Rels: make(map[string]RelMeta),
		Exts: make(map[string]ExtMeta),
	}
	if err := collectFields(t, nil, "", meta, map[reflect.Type]bool{t: true}); err != nil {
		return nil, err
	}
	modelCache.Store(t, meta)
	return meta, nil
}

// indirectType 解引用任意层级指针。
func indirectType(model any) reflect.Type {
	if model == nil {
		return nil
	}
	t := reflect.TypeOf(model)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// collectFields 递归收集 t 的字段元数据。
//
// 收录规则（文档 §6.2/§6.3 第 5 点）：
//   - Fields 仅收录打了 orm tag 的字段；匿名嵌入/extends 展开的叶子字段以
//     FieldIndex 反射索引路径平铺收录；打了 orm:"extends" 的嵌入字段本身以
//     Extends=true 标记条目收录（FieldIndex 定位嵌入来源字段，ExtendsPrefix
//     供 gormdriver 前缀 patch，文档 7.1）；
//   - Rels 收录打了 rel tag（或 gorm tag 含关联键，文档 5.3 兼容规则）的字段；
//   - Exts 收录打了 ext tag 的字段；
//   - 未导出字段跳过（嵌入类型必须导出，文档 2.2/2.3 实测双驱动均静默跳过）。
func collectFields(t reflect.Type, path []int, prefix string, meta *ModelMeta, visiting map[reflect.Type]bool) error {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" { // 未导出字段（含非导出嵌入类型，双驱动均静默跳过）
			continue
		}

		ormTag := strings.TrimSpace(sf.Tag.Get("orm"))
		fieldPath := append(append([]int{}, path...), i)

		var fm *FieldMeta
		if ormTag != "" {
			parsed, err := parseFieldTag(sf.Name, ormTag)
			if err != nil {
				return err
			}
			parsed.FieldName = sf.Name
			parsed.FieldIndex = fieldPath
			fm = &parsed
		}

		// 嵌入展开判定：匿名嵌入（无 orm tag 或 orm:"extends"）或显式 extends
		// 的 struct（含指针）字段递归展开；orm:"-" 的嵌入字段整组忽略不展开。
		deref := sf.Type
		for deref.Kind() == reflect.Ptr {
			deref = deref.Elem()
		}
		isEmbedTag := fm != nil && fm.Extends && !fm.Ignore
		shouldExpand := deref.Kind() == reflect.Struct &&
			((sf.Anonymous && (ormTag == "" || isEmbedTag)) ||
				(!sf.Anonymous && isEmbedTag))

		if shouldExpand {
			if visiting[deref] {
				return fmt.Errorf("ormtag: 字段 %q 存在循环嵌入（类型 %s）", sf.Name, deref)
			}
			innerPrefix := prefix
			if fm != nil {
				meta.Fields = append(meta.Fields, *fm) // 嵌入来源字段标记条目（Extends=true）
				innerPrefix = prefix + fm.ExtendsPrefix
			}
			visiting[deref] = true
			err := collectFields(deref, fieldPath, innerPrefix, meta, visiting)
			delete(visiting, deref)
			if err != nil {
				return err
			}
			continue
		}

		// 普通叶子字段：orm tag 收录进 Fields
		if fm != nil {
			// extends('前缀') 前缀应用到叶子字段显式列名（对齐 xorm ExtendsTagHandler
			// 的 col.Name = prefix + col.Name；无显式列名时由消费方组合嵌入标记条目
			// 的 ExtendsPrefix 与驱动命名约定）
			if prefix != "" && fm.ColumnName != "" {
				fm.ColumnName = prefix + fm.ColumnName
			}
			meta.Fields = append(meta.Fields, *fm)
		}

		// rel tag → gorm tag 兼容读取（文档 5.3：rel tag → gorm tag → 约定）
		if relTag := strings.TrimSpace(sf.Tag.Get("rel")); relTag != "" {
			rm, err := parseRelTag(sf.Name, relTag)
			if err != nil {
				return err
			}
			meta.Rels[sf.Name] = rm
		} else if rm, ok, err := parseGormAssocTag(sf.Name, sf.Tag.Get("gorm")); err != nil {
			return err
		} else if ok {
			meta.Rels[sf.Name] = rm
		}

		// ext tag（未知 ext key 不报错，留给驱动侧 Warn，文档 11.16）
		if extTag := strings.TrimSpace(sf.Tag.Get("ext")); extTag != "" {
			em, err := parseExtTag(sf.Name, extTag)
			if err != nil {
				return err
			}
			meta.Exts[sf.Name] = em
		}
	}
	return nil
}
