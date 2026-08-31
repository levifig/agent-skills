//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"errors"
	"os"
	"syscall"
)

func tryAcquireHarnessMutationFile(path string) (*os.File, bool, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, false, err
		}
		return nil, false, syscall.EINVAL
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return file, true, nil
}

func releaseHarnessMutationFile(_ string, file *os.File) error {
	if file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	return firstHookLockError(unlockErr, closeErr)
}

func harnessMutationLockContentionAdvice(_ string) string {
	return "the harness session may continue and retry on its next start"
}
