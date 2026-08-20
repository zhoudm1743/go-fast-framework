//go:build !windows

package fileStore

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func openAndLockFile(lockPath string, wait time.Duration) (*os.File, error) {
	if wait <= 0 {
		wait = defaultLockWait
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		_ = f.Close()
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("[GoFast] 获取跨进程锁超时: %s", lockPath)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func unlockFile(f *os.File) error {
	if f == nil {
		return nil
	}
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
	return err
}

func forceUnlockFile(lockPath string) error {
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
	err = os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
