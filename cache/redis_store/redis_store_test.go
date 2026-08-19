package redisStore

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/go-gorm/caches/v4"
)

// TestSerializeDeserialize_RoundTrip 验证 serialize/deserialize 各类值的往返保真。
func TestSerializeDeserialize_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		want any
	}{
		{"string", "hello"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"struct", map[string]any{"a": 1, "b": "x"}},
		{"nil", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := serialize(c.want)
			if err != nil {
				t.Fatalf("serialize(%v) 失败: %v", c.want, err)
			}
			if s[:2] != "j:" {
				t.Fatalf("非字节值应加 j: 前缀，实际 %q", s[:2])
			}
			got := deserialize(s)
			// 反序列化后与原始值 JSON 语义等价
			wantJSON, _ := json.Marshal(c.want)
			gotJSON, _ := json.Marshal(got)
			if !bytes.Equal(wantJSON, gotJSON) {
				t.Fatalf("往返不一致: want %s got %s", wantJSON, gotJSON)
			}
		})
	}
}

// TestSerializeDeserialize_BytesRoundTrip 验证 []byte 字节保真：
// 这是 Query().Cache() 在 redis store 下失效问题的核心回归用例。
func TestSerializeDeserialize_BytesRoundTrip(t *testing.T) {
	orig := []byte(`{"Dest":[{"id":"1","label":"XS"}],"RowsAffected":0}`)

	s, err := serialize(orig)
	if err != nil {
		t.Fatalf("serialize([]byte) 失败: %v", err)
	}
	if s[:2] != "b:" {
		t.Fatalf("字节值应加 b: 前缀，实际 %q", s[:2])
	}

	got, ok := deserialize(s).([]byte)
	if !ok {
		t.Fatalf("deserialize 应还原为 []byte，实际类型 %T", deserialize(s))
	}
	if !bytes.Equal(got, orig) {
		t.Fatalf("字节往返不一致:\n got %s\nwant %s", got, orig)
	}

	// 验证还原后的字节可直接被 caches.Query.Unmarshal 解析（原 bug 在此失败）
	var q caches.Query[any]
	if err := q.Unmarshal(got); err != nil {
		t.Fatalf("caches.Query.Unmarshal 应成功，实际失败: %v", err)
	}
}

// TestSerializeDeserialize_BytesNonUTF8 验证含任意二进制字节（含非法 UTF-8）的保真。
func TestSerializeDeserialize_BytesNonUTF8(t *testing.T) {
	orig := []byte{0xff, 0x00, 0x80, 0x7f, 0x01, '"', '\\', '\n', 0xfe}

	s, err := serialize(orig)
	if err != nil {
		t.Fatalf("serialize([]byte) 失败: %v", err)
	}
	got, ok := deserialize(s).([]byte)
	if !ok {
		t.Fatalf("deserialize 应还原为 []byte，实际类型 %T", deserialize(s))
	}
	if !bytes.Equal(got, orig) {
		t.Fatalf("二进制字节往返不一致: %v vs %v", got, orig)
	}
}

// TestSerializeDeserialize_EmptyBytes 验证空字节数组往返仍还原为 []byte（而非字符串）。
func TestSerializeDeserialize_EmptyBytes(t *testing.T) {
	s, err := serialize([]byte{})
	if err != nil {
		t.Fatalf("serialize(空字节) 失败: %v", err)
	}
	if s != "b:" {
		t.Fatalf("空字节应序列化为 b:，实际 %q", s)
	}
	got, ok := deserialize(s).([]byte)
	if !ok {
		t.Fatalf("空字节应还原为 []byte，实际类型 %T", deserialize(s))
	}
	if len(got) != 0 {
		t.Fatalf("空字节还原后长度应为 0，实际 %d", len(got))
	}
}

// TestDeserialize_LegacyData 验证无前缀的旧数据（旧版 serialize 落盘内容）仍可读，
// 保证升级期间存量缓存兼容。
func TestDeserialize_LegacyData(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want any
	}{
		{"json string", `"hello"`, "hello"},
		{"json number", `42`, float64(42)},
		{"json object", `{"a":1}`, map[string]any{"a": float64(1)}},
		{"raw string", `not-json`, `not-json`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantJSON, _ := json.Marshal(c.want)
			gotJSON, _ := json.Marshal(deserialize(c.in))
			if !bytes.Equal(wantJSON, gotJSON) {
				t.Fatalf("deserialize(%q) = %#v, want %#v", c.in, deserialize(c.in), c.want)
			}
		})
	}
}
