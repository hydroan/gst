package main

import "go/format"

func main() {}

func heal(src []byte) ([]byte, error) {
	return format.Source(src)
}
