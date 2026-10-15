package mock

import "testing"

// TestMockCache_TypedGettersMissingKey 回归测试：类型化 getter 对缺失键 + 默认值
// 不应 panic（此前 def 切片整体作为单个 any 传入 Get，类型断言 panic）。
func TestMockCache_TypedGettersMissingKey(t *testing.T) {
	m := NewMockCache()

	if got := m.GetInt("missing", 42); got != 42 {
		t.Fatalf("GetInt = %d, want 42", got)
	}
	if got := m.GetInt64("missing", 42); got != 42 {
		t.Fatalf("GetInt64 = %d, want 42", got)
	}
	if got := m.GetFloat64("missing", 1.5); got != 1.5 {
		t.Fatalf("GetFloat64 = %v, want 1.5", got)
	}
	if got := m.GetBool("missing", true); got != true {
		t.Fatalf("GetBool = %v, want true", got)
	}
	if got := m.GetString("missing", "d"); got != "d" {
		t.Fatalf("GetString = %q, want d", got)
	}
	// 无默认值时返回零值
	if got := m.GetInt64("missing"); got != 0 {
		t.Fatalf("GetInt64 = %d, want 0", got)
	}
}
