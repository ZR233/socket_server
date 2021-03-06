/*
@Time : 2019-08-26 16:24
@Author : zr
*/
package socket_server

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type HeaderHandler func(headerData []byte, ctx *Context) (bodyLen int, err error)
type OnConnect func(client *Client)

type Config struct {
	ListenIP string
	Port     int
	Handler  Handler
}

type Core struct {
	config       *Config
	clientPool   map[uint32]*Client
	clientPoolMu sync.Mutex
	idIter       *uint32
	netDeadLine  time.Duration
}

func (c *Core) GetClients() map[uint32]*Client {
	return c.clientPool
}

func NewCore(config *Config) *Core {
	idIter := uint32(0)

	c := &Core{
		clientPool:  make(map[uint32]*Client),
		netDeadLine: time.Second * 30,
	}
	c.config = config
	c.idIter = &idIter
	return c
}

func (c *Core) SetNetDeadLine(duration time.Duration) {
	c.netDeadLine = duration
}

func (c *Core) Run() {
	address := fmt.Sprintf("%s:%d", c.config.ListenIP, c.config.Port)
	tcpListen, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := tcpListen.Accept()
		if err != nil {
			logrus.Warn(err)
			continue
		}
		id := atomic.AddUint32(c.idIter, 1)
		client := newClient(conn, id, c, c.netDeadLine)

		c.config.Handler.OnConnect(client)
		if !client.Stopped() {
			c.addClient(client)
			go client.Run()
		}
	}
}

func (c *Core) addClient(client *Client) {
	c.clientPoolMu.Lock()
	defer c.clientPoolMu.Unlock()

	c.clientPool[client.id] = client
}

func (c *Core) deleteClient(client *Client) {
	c.clientPoolMu.Lock()
	defer c.clientPoolMu.Unlock()

	if client != nil {
		if _, ok := c.clientPool[client.id]; ok {
			delete(c.clientPool, client.id)
		}
	}
}
