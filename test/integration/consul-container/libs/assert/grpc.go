package assert

import "testing"

func GRPCPing(t *testing.T, addr string) {
	panic("TODO")
}

func FortioName(t *testing.T, name string, addr string, prefix string) {
	// HTTP Get of addr+prefix/debug?env=dump
	// grep for `^FORTIO_NAME=`
	// match to name
	panic("TODO")

}
