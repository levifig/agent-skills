//go:build windows

package cli

import "os"

// Windows uses a no-recovery exclusive-create lock. A crashed process may
// require manual cleanup, but no process may guess staleness and violate
// exclusivity.
func tryAcquireHarnessMutationFile(path string) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if os.IsExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return file, true, nil
}

func releaseHarnessMutationFile(path string, file *os.File) error {
	if file == nil {
		return nil
	}
	closeErr := file.Close()
	removeErr := os.Remove(path)
	if os.IsNotExist(removeErr) {
		removeErr = nil
	}
	return firstHookLockError(closeErr, removeErr)
}

func harnessMutationLockContentionAdvice(path string) string {
	return "the harness session may continue and retry on its next start; if no Loaf process remains and the refusal persists, remove the abandoned lock at " + path
}
