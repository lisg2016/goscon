package main

import (
	"bytes"
	crand "crypto/rand"
	"flag"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/ejoy/goscon/scp"
	"github.com/xjdrew/glog"
	"github.com/xtaci/kcp-go"

	"github.com/gorilla/websocket"
)

type ClientCase struct {
	connect string
}

func packetSize() int {
	sz := optMinPacket
	if optMaxPacket > optMinPacket {
		sz = sz + mrand.Intn(optMaxPacket-optMinPacket)
	}
	return sz
}

func (cc *ClientCase) testEchoWrite(conn net.Conn, times int, ch chan<- []byte, done chan<- error) {
	interval := time.Second / time.Duration(optPacketsPerSecond)
	for i := 0; i < times; i++ {
		sz := packetSize()
		buf := make([]byte, sz)
		crand.Read(buf[:sz])
		if _, err := conn.Write(buf[:sz]); err != nil {
			done <- err
		}
		ch <- buf[:sz]
		time.Sleep(interval)
	}
	close(ch)
	done <- nil
}

func (cc *ClientCase) testEchoRead(conn net.Conn, ch <-chan []byte, done chan<- error) {
	rbuf := make([]byte, optMaxPacket)
	for buf := range ch {
		sz := len(buf)
		if _, err := io.ReadFull(conn, rbuf[:sz]); err != nil {
			done <- err
		}
		if !bytes.Equal(buf[:sz], rbuf[:sz]) {
			done <- fmt.Errorf("echo unexpected<%d>:\nw:% x\nr:% x", sz, buf[:sz], rbuf[:sz])
		}
	}
	done <- nil
}

func (cc *ClientCase) testSCP(originConn *scp.Conn, conn net.Conn) (*scp.Conn, error) {
	sz := packetSize()
	wbuf := make([]byte, sz)
	rbuf := make([]byte, sz)
	crand.Read(wbuf)

	originConn.Write(wbuf[:sz/2])
	originConn.Write(wbuf[sz/2:])

	scon, err := scp.Client(conn, &scp.Config{ConnForReused: originConn})
	if err != nil {
		glog.Errorf("create reuse client failed: addr=%s, err=%s", conn.LocalAddr(), err.Error())
		return nil, err
	}
	originConn.Close()

	if _, err := io.ReadFull(scon, rbuf); err != nil {
		glog.Errorf("testSCP read scon failed: addr=%s, err=%s", conn.LocalAddr(), err.Error())
		return nil, err
	}

	if !bytes.Equal(wbuf, rbuf) {
		err := fmt.Errorf("testSCP<%s>:\nw:% x\nr:% x", scon.LocalAddr(), wbuf, rbuf)
		return nil, err
	}
	return scon, nil
}

func (cc *ClientCase) testN(conn *scp.Conn, packets int) error {
	ch := make(chan []byte, packets)
	done := make(chan error, 2)
	go cc.testEchoWrite(conn, packets, ch, done)
	go cc.testEchoRead(conn, ch, done)

	for i := 0; i < 2; i++ {
		err := <-done
		if err != nil {
			return err
		}
	}
	return nil
}

func Dial(network, connect string) (net.Conn, error) {
	if network == "tcp" {
		return net.Dial(network, connect)
	} else {
		return kcp.DialWithOptions(connect, nil, fecData, fecParity)
	}
}

func (cc *ClientCase) Start() error {
	n := optPackets / optReuses
	if n <= 0 {
		n = 1
	}

	raw, err := websocket.DefaultDialer.Dial(network, nil)
	if err != nil {
		glog.Errorf("dail failed: connect=%s, err=%s", cc.connect, err.Error())
		return err
	}
	preConn, _ := scp.Client(raw, nil)

	for i := 0; i < optReuses; i++ {
		if err = cc.testN(preConn, n); err != nil {
			glog.Errorf("testN failed: addr=%s, err=%s", preConn.LocalAddr(), err.Error())
			return err
		}

		new, err := Dial(network, cc.connect)
		if err != nil {
			glog.Errorf("dail failed: connect=%s, err=%s", cc.connect, err.Error())
			return err
		}

		nextConn, err := cc.testSCP(preConn, new)
		if err != nil {
			return err
		}
		preConn = nextConn
	}
	preConn.Close()
	return nil
}

var upgrader = websocket.Upgrader {
	ReadBufferSize:  64*1024,
	WriteBufferSize: 64*1024,
}
func startEchoServer(laddr string) error {
	go func() {
		http.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				glog.Errorln(err)
				return
			}

			go func(c *websocket.Conn) {
				defer c.Close()
				if optVerbose {
					for {
						b := make([]byte, 256)
						i, err := os.Stdout.Read(b)
						if err != nil {
							glog.Errorln(err)
							break
						}
						err2 := c.WriteMessage(websocket.BinaryMessage, b[:i])
						if err2 != nil {
							glog.Errorln(err2)
							break
						}
					}
				} else {
					for {
						_, buf, err := c.ReadMessage()
						if err != nil {
							glog.Errorln(err)
							break
						}
						err = c.WriteMessage(websocket.BinaryMessage, buf)
						if err != nil {
							glog.Errorln(err)
							break
						}
					}
				}
			}(conn)
		})
		http.ListenAndServe(laddr, nil)
	}()
	return nil
}

func testN() {
	ch := make(chan error, optConcurrent)
	for i := 0; i < optConcurrent; i++ {
		go func() {
			cc := &ClientCase{
				connect: optConnect,
			}
			ch <- cc.Start()
		}()
	}

	for i := 0; i < optConcurrent; i++ {
		err := <-ch
		if err != nil {
			glog.Errorf("<%d>: %s\n", i, err.Error())
		}
	}
}

var optConnect string
var optConcurrent, optPackets, optPacketsPerSecond, optMinPacket, optMaxPacket int
var optReuses int
var optRunRounds uint
var optVerbose bool
var network string
var fecData, fecParity int

func main() {
	// set default log directory
	flag.Set("log_dir", "./")
	flag.Set("logtostderr", "true")

	var echoServer string
	flag.IntVar(&optConcurrent, "concurrent", 1, "concurrent connections")
	flag.IntVar(&optPackets, "packets", 100, "total packets each connection")
	flag.IntVar(&optPacketsPerSecond, "pps", 100, "packets per second each connection")
	flag.IntVar(&optMinPacket, "min", 50, "min packet size")
	flag.IntVar(&optMaxPacket, "max", 100, "max packet size")
	flag.IntVar(&optReuses, "reuse", 1, "reuse times each connection")
	flag.UintVar(&optRunRounds, "rounds", 1, "run rounds")
	flag.StringVar(&echoServer, "startEchoServer", "", "start echo server")
	flag.StringVar(&optConnect, "connect", "127.0.0.1:1248", "connect to scon server")
	flag.BoolVar(&optVerbose, "verbose", false, "verbose")
	kcp := flag.NewFlagSet("kcp", flag.ExitOnError)
	kcp.IntVar(&fecData, "fec_data", 1, "FEC: number of shards to split the data into")
	kcp.IntVar(&fecParity, "fec_parity", 0, "FEC: number of parity shards")
	flag.Parse()

	if optMinPacket > optMaxPacket {
		optMinPacket, optMaxPacket = optMaxPacket, optMinPacket
	}

	args := flag.Args()

	if len(args) > 0 && args[0] == "kcp" {
		kcp.Parse(args[1:])
		network = "kcp"
	} else {
		network = "tcp"
	}

	if echoServer != "" {
		err := startEchoServer(echoServer)
		if err != nil {
			glog.Errorf("start echo server: %s", err.Error())
			return
		}
		glog.Info("run as echo server")
		// glog.Infof("listen %s", ln.Addr())
		ch := make(chan bool, 0)
		ch <- true
		return
	}

	go func() {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			glog.Errorf("listen failed: err=%v", err)
			return
		}
		glog.Infof("http listen: %s", ln.Addr())
		http.Serve(ln, nil)
	}()

	if optConnect != "" {
		glog.Info("run as echo client")
		glog.Infof("config: server=%s, concurrent=%d, packets=%d, pps=%d, sz=[%d, %d], reuses=%d",
			optConnect, optConcurrent, optPackets, optPacketsPerSecond, optMinPacket, optMaxPacket, optReuses)
		glog.Infof("run test %d rounds", optRunRounds)
		var round uint
		for round = 1; round <= optRunRounds; round++ {
			glog.Infof("round %d", round)
			testN()
		}
	}
}
