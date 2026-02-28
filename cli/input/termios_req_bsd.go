//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package input

import "golang.org/x/sys/unix"

func termiosRequests() (getReq, setReq uint) {
	return unix.TIOCGETA, unix.TIOCSETA
}
