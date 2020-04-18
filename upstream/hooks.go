package upstream

import (
	"net"

	"github.com/gorilla/websocket"
)

// Hook defines hook interface for upstream
type Hook interface {
	AfterConnected(local *websocket.Conn, remote net.Conn) (err error)
}

// installed hook
var upstreamHook Hook

func setHook(hook Hook) {
	if upstreamHook != nil {
		panic("setHook again")
	}
	upstreamHook = hook
}

// OnAfterConnected call when upstream connection is connected
func OnAfterConnected(local *websocket.Conn, remote net.Conn) (err error) {
	if upstreamHook == nil {
		return
	}
	err = upstreamHook.AfterConnected(local, remote)
	return
}
