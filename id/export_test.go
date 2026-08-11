// 此文件仅在测试编译时有效（文件名以 _test.go 结尾，包名为内部包名）。
// 通过导出内部符号，允许外部测试包（package id_test）模拟时间并验证内部状态。
package id

import "time"

// SetNowFn 覆盖时间源函数并重置内部序列状态，仅供测试使用。
// fn 返回当前 unix 毫秒时间戳。
func SetNowFn(fn func() int64) {
	mu.Lock()
	defer mu.Unlock()
	nowFn = fn
	lastMs = 0
	seq = 0
}

// ResetNowFn 恢复默认时间源并重置内部序列状态，仅供测试使用。
func ResetNowFn() {
	mu.Lock()
	defer mu.Unlock()
	nowFn = func() int64 { return time.Now().UnixMilli() }
	lastMs = 0
	seq = 0
}

// LastMs 返回当前内部记录的最近 ms 时间戳，仅供测试使用。
func LastMs() int64 {
	mu.Lock()
	defer mu.Unlock()
	return lastMs
}
