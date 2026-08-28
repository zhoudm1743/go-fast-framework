package utils

import "strings"

// MaskUtil 脱敏工具集。
var MaskUtil = maskUtil{}

type maskUtil struct{}

func (r maskUtil) Phone(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 7 {
		return s
	}
	return s[:3] + "****" + s[len(s)-4:]
}

func (r maskUtil) IDCard(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

func (r maskUtil) BankCard(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

func (r maskUtil) Email(s string) string {
	at := strings.Index(s, "@")
	if at <= 1 {
		return s
	}
	return s[:1] + "***" + s[at:]
}

func (r maskUtil) Name(s string) string {
	runes := []rune(s)
	if len(runes) <= 1 {
		return s
	}
	if len(runes) == 2 {
		return string(runes[0]) + "*"
	}
	return string(runes[0]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1])
}

func (r maskUtil) Address(s string) string {
	runes := []rune(s)
	if len(runes) <= 6 {
		return s
	}
	return string(runes[:6]) + strings.Repeat("*", len(runes)-6)
}
