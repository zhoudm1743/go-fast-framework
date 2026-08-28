package utils

import (
	"html"
	"regexp"
	"strings"
)

// HtmlUtil HTML 工具集（链式）。
var HtmlUtil = htmlUtil{}

type htmlUtil struct{}

type HTML struct {
	value string
}

func (r htmlUtil) Of(s string) *HTML {
	return &HTML{value: s}
}

func (r htmlUtil) Escape(s string) string {
	return html.EscapeString(s)
}

func (r htmlUtil) Unescape(s string) string {
	return html.UnescapeString(s)
}

func (r htmlUtil) StripTags(s string) string {
	return stripTagsRe.ReplaceAllString(s, "")
}

var stripTagsRe = regexp.MustCompile(`(?s)<[^>]*>`)

func (h *HTML) StripTags() *HTML {
	h.value = stripTagsRe.ReplaceAllString(h.value, "")
	return h
}

func (h *HTML) Escape() *HTML {
	h.value = html.EscapeString(h.value)
	return h
}

func (h *HTML) Unescape() *HTML {
	h.value = html.UnescapeString(h.value)
	return h
}

func (h *HTML) String() string {
	return h.value
}

func (h *HTML) Trim() *HTML {
	h.value = strings.TrimSpace(h.value)
	return h
}
