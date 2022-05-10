package sonic

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/talostrading/sonic/internal"
)

var _ File = &file{}

type file struct {
	ioc    *IO
	fd     int
	pd     internal.PollData
	closed uint32

	readDispatch, writeDispatch int
}

func Open(ioc *IO, path string, flags int, mode os.FileMode) (File, error) {
	fd, err := syscall.Open(path, flags, uint32(mode))
	if err != nil {
		return nil, err
	}

	f := &file{
		ioc: ioc,
		fd:  fd,
	}
	f.pd.Fd = fd
	return f, nil
}

func (f *file) Read(b []byte) (int, error) {
	n, err := syscall.Read(f.fd, b)
	if err != nil {
		if err == syscall.EWOULDBLOCK {
			return 0, ErrWouldBlock
		} else {
			if n == 0 {
				err = io.EOF
			}
		}
	}
	return n, err
}

func (f *file) Write(b []byte) (int, error) {
	n, err := syscall.Write(f.fd, b)
	if err != nil {
		if err == syscall.EWOULDBLOCK {
			return 0, ErrWouldBlock
		} else {
			if n == 0 {
				err = io.EOF
			}
		}
	}
	return n, err
}

func (f *file) AsyncRead(b []byte, cb AsyncCallback) {
	n, err := f.Read(b)
	if err == nil && n == len(b) {
		cb(nil, n)
		return
	}

	if err != nil && err != ErrWouldBlock {
		cb(err, 0)
		return
	}

	f.scheduleRead(b, cb)
}

func (f *file) scheduleRead(b []byte, cb AsyncCallback) {
	if f.Closed() {
		cb(io.EOF, 0)
		return
	}

	handler := f.getReadHandler(b, cb)
	f.pd.Set(internal.ReadEvent, handler)

	if err := f.setRead(); err != nil {
		cb(err, 0)
	} else {
		f.ioc.inflightReads[&f.pd] = struct{}{}
	}
}

func (f *file) getReadHandler(b []byte, cb AsyncCallback) internal.Handler {
	return func(err error) {
		delete(f.ioc.inflightReads, &f.pd)
		if err != nil {
			cb(err, 0)
		} else {
			fmt.Println("handler triggered")
			f.AsyncRead(b, cb)
		}
	}
}

func (f *file) setRead() error {
	return f.ioc.poller.SetRead(f.fd, &f.pd)
}

func (f *file) AsyncWrite(b []byte, cb AsyncCallback) {
	n, err := f.Write(b)
	if err == nil && n == len(b) {
		cb(nil, n)
		return
	}

	if err != nil && err != ErrWouldBlock {
		cb(err, 0)
	}

	f.scheduleWrite(b, cb)
}

func (f *file) scheduleWrite(b []byte, cb AsyncCallback) {
	if f.Closed() {
		cb(io.EOF, 0)
		return
	}

	handler := f.getWriteHandler(b, cb)
	f.pd.Set(internal.WriteEvent, handler)

	if err := f.setWrite(); err != nil {
		cb(err, 0)
	} else {
		f.ioc.inflightWrites[&f.pd] = struct{}{}
	}
}

func (f *file) getWriteHandler(b []byte, cb AsyncCallback) internal.Handler {
	return func(err error) {
		delete(f.ioc.inflightWrites, &f.pd)

		if err != nil {
			cb(err, 0)
		} else {
			f.AsyncWrite(b, cb)
		}
	}
}

func (f *file) setWrite() error {
	return f.ioc.poller.SetWrite(f.fd, &f.pd)
}

func (f *file) Close() error {
	if !atomic.CompareAndSwapUint32(&f.closed, 0, 1) {
		return io.EOF
	}

	err := f.ioc.poller.Del(f.fd, &f.pd) // TODO don't pass the fd as it's already in the PollData instance
	syscall.Close(f.fd)
	return err
}

func (f *file) Closed() bool {
	return atomic.LoadUint32(&f.closed) == 1
}

func (f *file) Seek(offset int64, whence SeekWhence) error {
	_, err := syscall.Seek(f.fd, offset, int(whence))
	return err
}
