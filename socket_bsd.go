//go:build darwin || netbsd || freebsd || openbsd || dragonfly

package sonic

import (
	"fmt"
	"net"
	"syscall"
)

func (s *Socket) BindToDevice(name string) error {
	iff, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}

	if s.domain == SocketDomainIPv4 {
		if err := syscall.SetsockoptInt(
			s.fd,
			syscall.IPPROTO_IP,
			syscall.IP_BOUND_IF,
			iff.Index,
		); err != nil {
			return err
		} else {
			s.boundInterface = iff
			return nil
		}
	} else {
		return fmt.Errorf("cannot yet bind to device when domain is ipv6")
	}
}

func (s *Socket) UnbindFromDevice() error {
	if s.boundInterface == nil {
		return nil
	}

	if s.domain == SocketDomainIPv4 {
		_, _, errno := syscall.Syscall6(
			uintptr(syscall.SYS_SETSOCKOPT),
			uintptr(s.fd),
			uintptr(syscall.IPPROTO_IP),
			uintptr(syscall.IP_BOUND_IF),
			0, 0, 0,
		)
		if errno != 0 {
			var err error
			err = errno
			return err
		} else {
			s.boundInterface = nil
			return nil
		}
	} else {
		return fmt.Errorf("cannot yet bind to device when domain is ipv6")
	}
}