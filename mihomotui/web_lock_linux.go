//go:build linux

package mihomotui

import (
	"os"
	"syscall"
)

func lockWebDeployment() (func(), error) {
	f, e := os.OpenFile("/run/lock/mihomo-web.lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
