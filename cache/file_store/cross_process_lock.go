package fileStore

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// defaultLockTTL 持锁进程崩溃后，锁文件可被回收的默认 TTL。
	defaultLockTTL = 30 * time.Second
	// defaultLockWait 获取锁的默认最长等待时间。
	defaultLockWait = 30 * time.Second
)

// acquireCrossProcessLock 尝试获取跨进程锁（非阻塞）。
func acquireCrossProcessLock(lockPath string, lockTTL time.Duration) (*os.File, bool, error) {
	if lockTTL <= 0 {
		lockTTL = defaultLockTTL
	}
	f, err := openAndLockFile(lockPath, 0)
	if err != nil {
		if isLockTimeout(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if err := writeLockMeta(f, lockTTL); err != nil {
		_ = unlockFile(f)
		_ = os.Remove(lockPath)
		return nil, false, err
	}
	return f, true, nil
}

// acquireCrossProcessLockBlocking 在 wait 超时前阻塞等待获取跨进程锁。
func acquireCrossProcessLockBlocking(lockPath string, wait, lockTTL time.Duration) (*os.File, error) {
	if lockTTL <= 0 {
		lockTTL = defaultLockTTL
	}
	f, err := openAndLockFile(lockPath, wait)
	if err != nil {
		return nil, err
	}
	if err := writeLockMeta(f, lockTTL); err != nil {
		_ = unlockFile(f)
		_ = os.Remove(lockPath)
		return nil, err
	}
	return f, nil
}

// releaseCrossProcessLock 释放跨进程锁。
func releaseCrossProcessLock(f *os.File, lockPath string) error {
	if err := unlockFile(f); err != nil {
		return err
	}
	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// forceReleaseCrossProcessLock 强制删除锁文件。
func forceReleaseCrossProcessLock(lockPath string) error {
	return forceUnlockFile(lockPath)
}

func writeLockMeta(f *os.File, lockTTL time.Duration) error {
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	expireAt := time.Now().Add(lockTTL).UnixNano()
	_, err := fmt.Fprintf(f, "%d\n%d\n", os.Getpid(), expireAt)
	return err
}

func isLockTimeout(err error) bool {
	return err != nil && strings.Contains(err.Error(), "获取跨进程锁超时")
}
