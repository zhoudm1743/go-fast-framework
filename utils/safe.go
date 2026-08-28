package utils

import (
	"context"
	"fmt"
	"time"
)

// SafeUtil 安全执行工具集。
var SafeUtil = safeUtil{}

type safeUtil struct{}

func (r safeUtil) Go(fn func()) {
	go func() {
		defer func() {
			_ = recover()
		}()
		fn()
	}()
}

func (r safeUtil) GoWithRecover(fn func() error) chan error {
	ch := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				ch <- fmt.Errorf("panic: %v", rec)
			}
		}()
		ch <- fn()
	}()
	return ch
}

func (r safeUtil) Run(fn func() (err error)) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic: %v", rec)
		}
	}()
	return fn()
}

func (r safeUtil) Timeout(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(c)
}
