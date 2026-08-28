package utils

import (
	"crypto/rand"
	"math/big"
	"reflect"
)

// RandomUtil 随机工具集。
var RandomUtil = randomUtil{}

type randomUtil struct{}

func (r randomUtil) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	max := big.NewInt(int64(n))
	v, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func (r randomUtil) IntRange(min, max int) int {
	if max <= min {
		return min
	}
	return min + r.Intn(max-min+1)
}

func (r randomUtil) Float64() float64 {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var n uint64
	for _, c := range b {
		n = n<<8 | uint64(c)
	}
	return float64(n) / float64(^uint64(0))
}

func (r randomUtil) Digits(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[r.Intn(10)]
	}
	return string(b)
}

func (r randomUtil) Letters(n int) string {
	return Random(n)
}

func (r randomUtil) WithCharset(n int, charset string) string {
	if charset == "" || n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[r.Intn(len(charset))]
	}
	return string(b)
}

func (r randomUtil) Pick(items any) (any, bool) {
	rv := reflectValueSlice(items)
	if !rv.IsValid() || rv.Len() == 0 {
		return nil, false
	}
	idx := r.Intn(rv.Len())
	return rv.Index(idx).Interface(), true
}

func reflectValueSlice(items any) reflect.Value {
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return reflect.Value{}
	}
	return rv
}
