//go:build windows

package fileStore

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var (
	modKernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modKernel32.NewProc("LockFileEx")
	procUnlockFileEx = modKernel32.NewProc("UnlockFileEx")
)

const (
	lockFileFailImmediately = 0x00000001
	lockFileExclusiveLock   = 0x00000002
)

type overlapped struct {
	internal uintptr
	internalHigh uintptr
	offset uint32
	offsetHigh uint32
	hEvent uintptr
}

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
		if lockWindowsFile(f, false) {
			return f, nil
		}
		_ = f.Close()
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("[GoFast] 获取跨进程锁超时: %s", lockPath)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func lockWindowsFile(f *os.File, block bool) bool {
	flags := uint32(lockFileExclusiveLock)
	if !block {
		flags |= lockFileFailImmediately
	}
	var ov overlapped
	r, _, _ := procLockFileEx.Call(
		f.Fd(),
		uintptr(flags),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ov)),
	)
	return r != 0
}

func unlockFile(f *os.File) error {
	if f == nil {
		return nil
	}
	var ov overlapped
	_, _, _ = procUnlockFileEx.Call(
		f.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ov)),
	)
	_ = f.Close()
	return nil
}

func forceUnlockFile(lockPath string) error {
	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
