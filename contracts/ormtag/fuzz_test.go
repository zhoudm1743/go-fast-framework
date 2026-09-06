package ormtag

import (
	"reflect"
	"strings"
	"testing"
)

// FuzzParseField 随机 token 串解析不 panic；合法输入解析幂等（文档 11.19/R6）。
//
// go test（无 -fuzz）默认运行种子语料作为常规用例。
func FuzzParseField(f *testing.F) {
	seeds := []string{
		"pk varchar(16) 'id'",
		"'id' varchar(16) pk",
		"varchar(100) 'name' notnull default('') comment('姓名')",
		"default 0",
		`default 'act ive'`,
		"default('')",
		"decimal(10,2) 'amount'",
		"index(idx_org)",
		"unique(uk_tenant_phone)",
		`extends('author_')`,
		"created 'created_at'",
		"->",
		"<-",
		"-",
		"not null",
		"not",
		"cache",
		"nocache",
		"deleted",
		"serializer:json",
		"pkk",
		"varchar(100) user_name",
		`varchar(10) 'unclosed`,
		"varchar(100",
		"decimal(10,2),",
		"decimal(1(2))",
		"collate utf8mb4_bin",
		"collate('utf8mb4_bin')",
		"jsonb",
		"'a''b'",
		"''",
		"  ",
		"'act ive' 'col name' default 'x y' comment('it''s')",
		"INT8 UNSIGNED BIGSERIAL ARRAY XML MONEY",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, tag string) {
		// Go struct tag 语法限制：注入值不得包含引号定界符与反引号
		if strings.ContainsAny(tag, "\"`") || strings.Contains(tag, "\n") {
			t.Skip()
		}

		// 任何输入不得 panic
		meta, err := parseFieldTag("F", tag)
		_ = meta

		// 合法输入解析幂等：两次解析结果一致
		if err == nil {
			again, err2 := parseFieldTag("F", tag)
			if err2 != nil {
				t.Fatalf("同一输入两次解析结果不一致（首次成功，再次报错 %v）", err2)
			}
			if !reflect.DeepEqual(meta, again) {
				t.Fatalf("解析非幂等:\n第一次 %+v\n第二次 %+v", meta, again)
			}
		}

		// 模型级链路冒烟：可解析的 tag 经 reflect.StructOf 构造模型后
		// Parse 全链路同样不 panic（忽略解析错误——单字段合法性由上文判定）
		st := reflect.StructOf([]reflect.StructField{{
			Name: "F",
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(`orm:"` + tag + `"`),
		}})
		_, _ = Parse(reflect.New(st).Interface())
	})
}

// FuzzSplitTag token 化状态机专项：不 panic 且原始串可复原语义。
func FuzzSplitTag(f *testing.F) {
	for _, s := range seeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, tag string) {
		tokens, err := splitTag(strings.TrimSpace(tag))
		if err != nil {
			return
		}
		// 无错误时：token 原文必须来自输入串（raw span 正确性）
		for _, tok := range tokens {
			if tok.raw == "" {
				t.Fatalf("token raw 不应为空: %#v (输入 %q)", tok, tag)
			}
			if !strings.Contains(tag, tok.raw) {
				t.Fatalf("token raw %q 不属于输入 %q", tok.raw, tag)
			}
			// 注：引号 token 携带括号参数（'a'(b)）由 parseFieldTag 分发层报错，
			// token 化层容忍，不在此断言。
		}
	})
}

func seeds() []string {
	return []string{
		"pk varchar(16) 'id'",
		"default 'act ive'",
		"comment('it''s')",
		"decimal(10,2)",
		"'''",
		"(((",
		")))",
		",,",
		"a'b'c",
		"'",
	}
}
