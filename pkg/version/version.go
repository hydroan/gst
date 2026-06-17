// Package pkgversion provides version string helpers; the name avoids conflicting
// with the standard library "runtime/version" package.
package pkgversion

import (
	"strconv"
	"strings"
)

const version = "0.0.1"

var (
	PkgnameServer = "server"
	PkgnameClient = "client"
)

func Full() string {
	return version
}

func Major(v string) int64 {
	return getSubVersion(v, 1)
}

func Minor(v string) int64 {
	return getSubVersion(v, 2)
}

func getSubVersion(v string, position int) int64 {
	arr := strings.Split(v, ".")
	if len(arr) < 3 {
		return 0
	}
	res, _ := strconv.ParseInt(arr[position], 10, 64)
	return res
}

func ServerName() string {
	return PkgnameServer
}

func ClientName() string {
	return PkgnameClient
}
