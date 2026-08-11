// Package id 提供 GoFast 框架内置的时序 ID 生成器。
//
// # 规格
//
//   - 长度：16 字符（固定）
//   - 字符集：Crockford Base32 小写（去掉 i l o u，避免视觉混淆）：
//     0 1 2 3 4 5 6 7 8 9 a b c d e f g h j k m n p q r s t v w x y z
//   - 结构：[10 chars | 50-bit 毫秒时间戳] [6 chars | 30-bit 单调序列]
//   - 字典序 ≡ 时间序：可直接在数据库 ORDER BY id ASC/DESC 按创建时间排序
//   - 并发安全：全局 Mutex 保护，同一毫秒内序列单调递增
//   - 时钟回拨安全：lastMs 只增不减，NTP 调整后 ID 仍单调递增
//   - 序列溢出保护：同毫秒内超过 2^30 次后自动推进虚拟时钟
//   - 无额外依赖
//
// # 容量
//
//   - 时间戳：2^50 ms ≈ 35,000 年（绝对够用）
//   - 序列：2^30 ≈ 10 亿/ms（实际请求量永远不会触及溢出）
//
// # 示例
//
//	id.New() // "01jdm4qr0s2fgk01"（16 chars，每次不同）
package id

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Size 是生成 ID 的固定字符长度。
const Size = 16

// charset 为 Crockford Base32 小写（32 字符，去掉 i l o u）。
const charset = "0123456789abcdefghjkmnpqrstvwxyz"

// charToVal 将有效 ASCII 字节映射到 0-31 索引，无效字符返回 -1。
var charToVal [256]int8

func init() {
	for i := range charToVal {
		charToVal[i] = -1
	}
	for i, c := range charset {
		charToVal[byte(c)] = int8(i)
	}
}

var (
	mu     sync.Mutex
	lastMs int64  // 最近使用的 ms 时间戳（只增不减，处理时钟回拨）
	seq    uint32 // 当前 ms 内的序列号（30-bit）
	nowFn  = func() int64 { return time.Now().UnixMilli() }
)

// New 生成一个 16 字符的时序 ID，并发安全。
//
// 格式（大端，高位在左，字典序 == 时间序）：
//
//	chars  0–9  : 50-bit 毫秒时间戳
//	chars 10–15 : 30-bit 单调序列
//
// 时钟回拨时使用上次时间戳继续递增；序列耗尽时推进虚拟时钟。
func New() string {
	mu.Lock()
	now := nowFn()
	if now > lastMs {
		// 新毫秒（正常推进）：重置序列为随机值，避免暴露全局计数器。
		lastMs = now
		seq = randUint30()
	} else {
		// 相同毫秒或时钟回拨：序列单调递增，保证全局有序。
		seq = (seq + 1) & 0x3FFFFFFF
		if seq == 0 {
			// 序列耗尽（同 ms 超过 10 亿次，极端罕见）：推进虚拟时钟。
			lastMs++
			seq = randUint30()
		}
	}
	ts := uint64(lastMs) & ((uint64(1) << 50) - 1)
	r := seq
	mu.Unlock()

	return encode(ts, r)
}

// normalizeReplacer 将 Crockford Base32 输入归一化：
// 移除连字符、纠正常见混淆字符（I/i/L/l → 1、O/o → 0）。
var normalizeReplacer = strings.NewReplacer(
	"-", "",
	"I", "1", "i", "1",
	"L", "1", "l", "1",
	"O", "0", "o", "0",
)

// Parse 解析 ID 中嵌入的毫秒时间戳。
// 支持 Crockford Base32 大小写不敏感解析，自动纠正常见混淆字符（I/i/L/l→1、O/o→0），
// 并忽略连字符以增强可读性。
func Parse(v string) (time.Time, error) {
	// 归一化：移除连字符、纠错字符映射、转小写
	v = strings.ToLower(normalizeReplacer.Replace(v))
	if len(v) != Size {
		return time.Time{}, fmt.Errorf("id: 无效长度 %d（应为 %d）", len(v), Size)
	}
	var ts uint64
	for i := 0; i < 10; i++ {
		idx := charToVal[v[i]]
		if idx < 0 {
			return time.Time{}, fmt.Errorf("id: 位置 %d 存在无效字符 %q", i, v[i])
		}
		ts = ts<<5 | uint64(idx)
	}
	return time.UnixMilli(int64(ts)), nil
}

// encode 将 50-bit 时间戳和 30-bit 序列编码为 16 字符大端字符串。
func encode(ts50 uint64, r30 uint32) string {
	var b [Size]byte
	for i := 9; i >= 0; i-- {
		b[i] = charset[ts50&0x1F]
		ts50 >>= 5
	}
	for i := 15; i >= 10; i-- {
		b[i] = charset[r30&0x1F]
		r30 >>= 5
	}
	return string(b[:])
}

// randUint30 从 crypto/rand 读取 30-bit 随机数；失败时退化为时间 hash。
func randUint30() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint32(time.Now().UnixNano()) & 0x3FFFFFFF
	}
	return binary.BigEndian.Uint32(b[:]) & 0x3FFFFFFF
}
