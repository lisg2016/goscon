package main

import (
	"time"
	"fmt"
	"net"
	"errors"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/xjdrew/glog"
	"github.com/ejoy/goscon/scp"
)

type wsConn struct {
	*websocket.Conn
	readTimeout time.Duration
}

func (p *wsConn) Read(b []byte) (int, error) {
	if p.readTimeout > 0 {
		p.SetReadDeadline(time.Now().Add(p.readTimeout))
	}
	_, message, err := p.ReadMessage()
	if err != nil {
		return 0, err
	}

	if len(b) < len(message) {
		return 0, errors.New(fmt.Sprintf("ws read buffer error: %d msg length: %d", len(b), len(message)))
	}

	copy(b, message)
	return len(message), nil
}

func (p *wsConn) Write(b []byte) (int, error) {
	err := p.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return  0, err
	}
	return len(b), nil
}

func (p *wsConn) RemoteAddr() net.Addr {
	return p.RemoteAddr()
}

func (p *wsConn) LocalAddr() net.Addr {
	return p.LocalAddr()
}

func (p *wsConn) SetDeadline(t time.Time) error {
	return p.SetDeadline(t)
}
func (p *wsConn) SetReadDeadline(t time.Time) error {
	return p.SetReadDeadline(t)
}
func (p *wsConn) SetWriteDeadline(t time.Time) error {
	return p.SetWriteDeadline(t)
}
func (p *wsConn) Close() error {
	return p.Close()
}


type WSListener struct {
	connCh	chan *websocket.Conn
	addr	string
}

func (p *WSListener) onAccept(conn *websocket.Conn) {
	p.connCh <- conn
}

func (p *WSListener) Accept() (scp.ConnInterface, error) {
	conn := <- p.connCh

	if glog.V(1) {
		glog.Infof("accept new ws connection: addr=%s", conn.RemoteAddr())
	}
	ws := &wsConn {
		conn,
		60*3,
	}
	return ws, nil
}

func (p *WSListener) Addr() string {
	return p.addr
}

func (p *WSListener) Close() error {
	return nil
}

var upgrader = websocket.Upgrader {
	ReadBufferSize:  64*1024,
	WriteBufferSize: 64*1024,
}

func NewWsListener(laddr string) (*WSListener, error) {
	ws := &WSListener {
		addr: laddr,
		connCh: make(chan *websocket.Conn, 256),
	}

	go func() {
		http.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				glog.Errorln(err)
				return
			}
			ws.onAccept(conn)
		})
		glog.Fatal(http.ListenAndServe(laddr, nil))
	}()

	return ws, nil
}
