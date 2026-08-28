package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MoneyUtil 金额工具集（链式，内部 int64 分）。
var MoneyUtil = moneyUtil{}

type moneyUtil struct{}

type Money struct {
	fen int64
}

func (r moneyUtil) OfFen(fen int64) *Money {
	return &Money{fen: fen}
}

func (r moneyUtil) OfYuan(yuan float64) *Money {
	return &Money{fen: int64(math.Round(yuan * 100))}
}

func (r moneyUtil) YuanToFen(yuan float64) int64 {
	return int64(math.Round(yuan * 100))
}

func (r moneyUtil) FenToYuan(fen int64) float64 {
	return float64(fen) / 100
}

func (m *Money) AddFen(n int64) *Money {
	m.fen += n
	return m
}

func (m *Money) AddYuan(y float64) *Money {
	m.fen += int64(math.Round(y * 100))
	return m
}

func (m *Money) Mul(n int64) *Money {
	m.fen *= n
	return m
}

func (m *Money) Neg() *Money {
	m.fen = -m.fen
	return m
}

func (m *Money) Fen() int64 {
	return m.fen
}

func (m *Money) Yuan() float64 {
	return float64(m.fen) / 100
}

func (m *Money) FormatYuan() string {
	y := float64(m.fen) / 100
	s := strconv.FormatFloat(y, 'f', 2, 64)
	parts := strings.Split(s, ".")
	intPart := parts[0]
	sign := ""
	if strings.HasPrefix(intPart, "-") {
		sign = "-"
		intPart = intPart[1:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if len(parts) > 1 {
		return sign + b.String() + "." + parts[1]
	}
	return sign + b.String()
}

func (m *Money) UpperCNY() string {
	return upperCNY(m.fen)
}

var cnNums = []string{"零", "壹", "贰", "叁", "肆", "伍", "陆", "柒", "捌", "玖"}
var cnUnits = []string{"", "拾", "佰", "仟"}
var cnBigUnits = []string{"", "万", "亿", "兆"}

func upperCNY(fen int64) string {
	if fen == 0 {
		return "零元整"
	}
	neg := fen < 0
	if neg {
		fen = -fen
	}
	yuan := fen / 100
	jiao := (fen / 10) % 10
	f := fen % 10

	var sb strings.Builder
	if neg {
		sb.WriteString("负")
	}
	sb.WriteString(convertInteger(yuan))
	sb.WriteString("元")
	if jiao == 0 && f == 0 {
		sb.WriteString("整")
	} else {
		if jiao > 0 {
			sb.WriteString(cnNums[jiao])
			sb.WriteString("角")
		} else if yuan > 0 {
			sb.WriteString("零")
		}
		if f > 0 {
			sb.WriteString(cnNums[f])
			sb.WriteString("分")
		}
	}
	return sb.String()
}

func convertInteger(n int64) string {
	if n == 0 {
		return cnNums[0]
	}
	s := fmt.Sprintf("%d", n)
	var parts []string
	for len(s) > 0 {
		chunk := s
		if len(s) > 4 {
			chunk = s[len(s)-4:]
			s = s[:len(s)-4]
		} else {
			s = ""
		}
		parts = append([]string{chunk}, parts...)
	}
	var sb strings.Builder
	for i, chunk := range parts {
		chunkInt, _ := strconv.Atoi(chunk)
		if chunkInt == 0 {
			continue
		}
		sb.WriteString(chunkToCN(chunk))
		sb.WriteString(cnBigUnits[len(parts)-1-i])
	}
	return sb.String()
}

func chunkToCN(chunk string) string {
	var sb strings.Builder
	zero := false
	for i, c := range chunk {
		d := int(c - '0')
		pos := len(chunk) - 1 - i
		if d == 0 {
			zero = true
			continue
		}
		if zero {
			sb.WriteString(cnNums[0])
			zero = false
		}
		sb.WriteString(cnNums[d])
		sb.WriteString(cnUnits[pos])
	}
	return sb.String()
}
