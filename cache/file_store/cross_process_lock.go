package fileStore

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// defaultLockTTL 写入锁文件的元信息 TTL（仅作排查参考）。
	// flock/LockFileEx 在进程退出或 fd 关闭时由内核自动释放，无需依赖 TTL 回收。
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

// releaseCrossProcessLock 释放跨进程锁：仅解锁关闭，不删除锁文件。
// flock/LockFileEx 基于 inode（Linux 为 open file description），删除基于路径——
// "解锁后立即删文件"会让重试中的等待者 flock 到已断链的旧 inode、后来者经
// O_CREATE 获得新 inode，两个持锁方并发进入临界区（经典 flock+unlink 双持
// 竞态，曾致跨进程并发自增丢更新）。锁文件持久存在无副作用：进程退出/崩溃
// 时内核自动释放锁；Flush 也会保留 .locks 目录（见 file_store.go Flush）。
func releaseCrossProcessLock(f *os.File, _ string) error {
	return unlockFile(f)
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
