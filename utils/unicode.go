package utils

import (
	"strings"
	"unicode"
)

// UnicodeUtil Unicode 工具集（链式）。
var UnicodeUtil = unicodeUtil{}

type unicodeUtil struct{}

type Unicode struct {
	value string
}

func (r unicodeUtil) Of(s string) *Unicode {
	return &Unicode{value: s}
}

func (u *Unicode) ToFullWidth() *Unicode {
	var b strings.Builder
	for _, r := range u.value {
		switch {
		case r == ' ':
			b.WriteRune('\u3000')
		case r >= '!' && r <= '~':
			b.WriteRune(r - '!' + '\uFF01')
		default:
			b.WriteRune(r)
		}
	}
	u.value = b.String()
	return u
}

func (u *Unicode) ToHalfWidth() *Unicode {
	var b strings.Builder
	for _, r := range u.value {
		switch {
		case r == '\u3000':
			b.WriteRune(' ')
		case r >= '\uFF01' && r <= '\uFF5E':
			b.WriteRune(r - '\uFF01' + '!')
		default:
			b.WriteRune(r)
		}
	}
	u.value = b.String()
	return u
}

func (u *Unicode) TrimSpace() *Unicode {
	u.value = strings.TrimSpace(u.value)
	u.value = strings.Trim(u.value, "\u3000")
	return u
}

func (u *Unicode) String() string {
	return u.value
}

func (u *Unicode) HasHan() bool {
	for _, r := range u.value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func (u *Unicode) HanCount() int {
	n := 0
	for _, r := range u.value {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n
}

func (u *Unicode) DisplayWidth() int {
	w := 0
	for _, r := range u.value {
		if r < 128 {
			w++
		} else {
			w += 2
		}
	}
	return w
}
