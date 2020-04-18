// Package scp implements Stable Connection Protocol
package scp

import (
	"net"
	"time"
)

type Accept interface {
	Accept() (conn ConnInterface, err error)
	Addr() string
	Close() error
}

type ConnInterface interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}


type SCPServer interface {
	// allocate a id for connection
	AcquireID() int

	// release a id if handshake failed
	ReleaseID(id int)

	// query a conneciton by id
	QueryByID(id int) *Conn
}

type Config struct {
	// preferred target server
	// for client
	TargetServer string

	// reused conn
	// for client
	ConnForReused *Conn

	// SCPServer
	// for server
	ScpServer SCPServer
}

var defaultConfig = &Config{}

func (config *Config) clone() *Config {
	return &Config{
		ScpServer: config.ScpServer,
	}
}

func Server(conn ConnInterface, config *Config) *Conn {
	if config.ScpServer == nil {
		panic("config.ScpServer == nil")
	}

	c := &Conn{
		conn:   conn,
		config: config.clone(),
	}

	return c
}

// Client wraps conn as scp.Conn
func Client(conn ConnInterface, config *Config) (*Conn, error) {
	if config == nil {
		config = defaultConfig
	}

	c := &Conn{
		conn:   conn,
		config: config,
	}

	if config.ConnForReused != nil {
		if !config.ConnForReused.spawn(c) {
			return nil, ErrNotAcceptable
		}
		c.handshakes = config.ConnForReused.handshakes + 1
	}
	return c, nil
}
