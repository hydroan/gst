package util

import (
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	"github.com/cockroachdb/errors"
)

func GetConnection(conn net.Conn) Connection {
	if conn == nil {
		return *new(Connection)
	}
	var (
		rip, lip     string
		rport, lport int
	)
	laddr := strings.Split(conn.LocalAddr().String(), ":")
	if len(laddr) == 2 {
		lip = laddr[0]
		if port, err := strconv.Atoi(laddr[1]); err == nil {
			lport = port
		}
	}
	raddr := strings.Split(conn.RemoteAddr().String(), ":")
	if len(raddr) == 2 {
		rip = raddr[0]
		if port, err := strconv.Atoi(raddr[1]); err == nil {
			rport = port
		}
	}
	return Connection{
		RemoteIP:   rip,
		LocalIP:    lip,
		RemotePort: rport,
		LocalPort:  lport,
	}
}

type Connection struct {
	RemoteIP   string
	LocalIP    string
	RemotePort int
	LocalPort  int
}

// GetFdFromConn get net.Conn's file descriptor.
func GetFdFromConn(c net.Conn) int {
	v := reflect.Indirect(reflect.ValueOf(c))
	conn := v.FieldByName("conn")
	netFD := reflect.Indirect(conn.FieldByName("fd"))
	pfd := netFD.FieldByName("pfd")
	fd := int(pfd.FieldByName("Sysfd").Int())
	return fd
}

// GetFdFromListener get net.Listener's file descriptor.
func GetFdFromListener(l net.Listener) int {
	v := reflect.Indirect(reflect.ValueOf(l))
	netFD := reflect.Indirect(v.FieldByName("fd"))
	pfd := netFD.FieldByName("pfd")
	fd := int(pfd.FieldByName("Sysfd").Int())
	return fd
}

func IsConnClosed(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	return false
}
