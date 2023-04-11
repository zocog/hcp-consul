package internal

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
)

var ErrPipeClosed = errors.New("the pipe has been closed")

type PipeListener struct {
	connections chan net.Conn
	closed      atomic.Bool
	closeCh     chan struct{}
}

func ListenPipe() *PipeListener {
	return &PipeListener{
		connections: make(chan net.Conn),
		closeCh:     make(chan struct{}),
	}
}

func (p *PipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-p.connections:
		return conn, nil
	case <-p.closeCh:
		return nil, ErrPipeClosed
	}
}

func (p *PipeListener) Close() error {
	if p.closed.CompareAndSwap(false, true) {
		close(p.closeCh)
	}
	return nil
}

func (p *PipeListener) DialContext(ctx context.Context, _ string, _ string) (net.Conn, error) {
	// check if we have been closed in the past
	if p.closed.Load() {
		return nil, ErrPipeClosed
	}

	// create the server and client side of the connection
	serverConn, clientConn := net.Pipe()

	select {
	case <-ctx.Done():
		serverConn.Close()
		clientConn.Close()
		return nil, ctx.Err()
	case <-p.closeCh:
		serverConn.Close()
		clientConn.Close()
		return nil, ErrPipeClosed
	// Send the server connection to whatever is accepting connections from the PipeListener.
	// This will block until something has accepted the conn.
	case p.connections <- serverConn:
		return clientConn, nil
	}
}

func (p *PipeListener) DialContextWithoutNetwork(ctx context.Context, addr string) (net.Conn, error) {
	return p.DialContext(ctx, "pipe", addr)
}

func (p *PipeListener) Addr() net.Addr {
	return &pipeAddr{}
}

type pipeAddr struct{}

func (*pipeAddr) Network() string {
	return "pipe"
}

func (*pipeAddr) String() string {
	return "pipe"
}
