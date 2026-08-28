package utils

import (
	"fmt"
	"math"
	"strconv"
)

// NumberUtil 数值工具集（链式）。
var NumberUtil = numberUtil{}

type numberUtil struct{}

// Num 链式数值对象。
type Num struct {
	value float64
	asInt bool
	intVal int64
}

func (r numberUtil) Of(n float64) *Num {
	return &Num{value: n}
}

func (r numberUtil) OfInt(n int64) *Num {
	return &Num{value: float64(n), asInt: true, intVal: n}
}

func (n *Num) Abs() *Num {
	if n.asInt {
		if n.intVal < 0 {
			n.intVal = -n.intVal
		}
		n.value = float64(n.intVal)
	} else if n.value < 0 {
		n.value = -n.value
	}
	return n
}

func (n *Num) Round(precision int) *Num {
	p := math.Pow(10, float64(precision))
	n.value = math.Round(n.value*p) / p
	n.asInt = false
	return n
}

func (n *Num) Ceil() *Num {
	n.value = math.Ceil(n.value)
	n.asInt = false
	return n
}

func (n *Num) Floor() *Num {
	n.value = math.Floor(n.value)
	n.asInt = false
	return n
}

func (n *Num) Clamp(min, max float64) *Num {
	if n.value < min {
		n.value = min
	}
	if n.value > max {
		n.value = max
	}
	n.asInt = false
	return n
}

func (n *Num) Percent(total float64) *Num {
	if total == 0 {
		n.value = 0
	} else {
		n.value = n.value / total * 100
	}
	return n
}

func (n *Num) InRange(min, max float64) bool {
	return n.value >= min && n.value <= max
}

func (n *Num) IsEven() bool {
	return int64(n.value)%2 == 0
}

func (n *Num) IsOdd() bool {
	return !n.IsEven()
}

func (n *Num) Float() float64 {
	return n.value
}

func (n *Num) Int() int64 {
	if n.asInt {
		return n.intVal
	}
	return int64(n.value)
}

func (n *Num) String() string {
	return strconv.FormatFloat(n.value, 'f', -1, 64)
}

func (r numberUtil) Sum(nums ...float64) float64 {
	var s float64
	for _, n := range nums {
		s += n
	}
	return s
}

func (r numberUtil) Avg(nums ...float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	return r.Sum(nums...) / float64(len(nums))
}

func (r numberUtil) Min(nums ...float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	m := nums[0]
	for _, n := range nums[1:] {
		if n < m {
			m = n
		}
	}
	return m
}

func (r numberUtil) Max(nums ...float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	m := nums[0]
	for _, n := range nums[1:] {
		if n > m {
			m = n
		}
	}
	return m
}

func (r numberUtil) HumanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (r numberUtil) Range(start, end int) []int {
	if end < start {
		return nil
	}
	out := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, i)
	}
	return out
}

func (r numberUtil) Clamp(v, min, max float64) float64 {
	return r.Of(v).Clamp(min, max).Float()
}

func (r numberUtil) InRange(v, min, max float64) bool {
	return r.Of(v).InRange(min, max)
}
