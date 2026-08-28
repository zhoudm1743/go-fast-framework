package utils

import (
	"os"
	"strings"
)

// ColorUtil CLI 彩色输出工具集（链式）。
var ColorUtil = colorUtil{enabled: isTTY()}

type colorUtil struct {
	enabled bool
}

type Color struct {
	value   string
	enabled bool
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func (r colorUtil) Of(s string) *Color {
	return &Color{value: s, enabled: r.enabled}
}

func (r colorUtil) Success(s string) string {
	return r.Of(s).Green().String()
}

func (r colorUtil) Error(s string) string {
	return r.Of(s).Red().String()
}

func (r colorUtil) Warn(s string) string {
	return r.Of(s).Yellow().String()
}

func (r colorUtil) Info(s string) string {
	return r.Of(s).Cyan().String()
}

func (c *Color) wrap(code string) *Color {
	if c.enabled {
		c.value = code + c.value + "\033[0m"
	}
	return c
}

func (c *Color) Red() *Color     { return c.wrap("\033[31m") }
func (c *Color) Green() *Color   { return c.wrap("\033[32m") }
func (c *Color) Yellow() *Color  { return c.wrap("\033[33m") }
func (c *Color) Blue() *Color    { return c.wrap("\033[34m") }
func (c *Color) Cyan() *Color    { return c.wrap("\033[36m") }
func (c *Color) Bold() *Color    { return c.wrap("\033[1m") }

func (c *Color) String() string {
	return c.value
}

func (c *Color) Concat(parts ...string) *Color {
	c.value += strings.Join(parts, "")
	return c
}
