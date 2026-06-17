package util

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnection(t *testing.T) {
	conn, err := net.Dial("tcp", "8.8.8.8:53")
	require.NoError(t, err)
	defer conn.Close()
	c := GetConnection(conn)
	fmt.Printf("%+v\n", c)
}

func TestGetFdFromConn(t *testing.T) {
	conn, err := net.Dial("tcp", "8.8.8.8:53")
	require.NoError(t, err)
	defer conn.Close()
	fd := GetFdFromConn(conn)
	fmt.Println(fd)
}

func TestGetFdFromListener(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:12345")
	require.NoError(t, err)
	defer l.Close()
	fd := GetFdFromListener(l)
	fmt.Println(fd)
}
