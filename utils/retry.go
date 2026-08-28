package utils

import (
	"context"
	"time"
)

// RetryUtil 重试工具集。
var RetryUtil = retryUtil{}

type retryUtil struct{}

func (r retryUtil) Do(ctx context.Context, fn func() error, attempts int, backoff time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if err = fn(); err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		wait := backoff * time.Duration(i+1)
		if ctx != nil {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		} else {
			time.Sleep(wait)
		}
	}
	return err
}

func (r retryUtil) DoWithJitter(ctx context.Context, fn func() error, attempts int, backoff time.Duration) error {
	return r.Do(ctx, fn, attempts, backoff)
}
