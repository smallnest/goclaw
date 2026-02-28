//go:build linux

package input

import "golang.org/x/sys/unix"

func termiosRequests() (getReq, setReq uint) {
	return unix.TCGETS, unix.TCSETS
}
