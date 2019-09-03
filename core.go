/*
@Time : 2019-08-26 16:24
@Author : zr
*/
package socket_server

import (
	"fmt"
	"github.com/ZR233/socket_server/handler"
	"net"
	"sync/atomic"
)

type HeaderHandler func(headerData []byte, ctx *handler.Context) (bodyLen int, err error)
type OnConnect func(client *Client)

type Config struct {
	ListenIP string
	Port     int
	Handler  handler.Handler
}

type Logger interface {
	Warn(msg ...interface{})
	Info(msg ...interface{})
	Debug(msg ...interface{})
}

type Core struct {
	config     *Config
	clientPool map[uint32]*Client
	logger     Logger
	idIter     *uint32
}

func NewCore(config *Config) *Core {
	idIter := uint32(0)

	c := &Core{}
	c.config = config
	c.idIter = &idIter
	return c
}

func (c *Core) SetLogger(logger Logger) {
	c.logger = logger
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
			c.logger.Warn(err)
			continue
		}
		client := newClient(conn, c)
		id := atomic.AddUint32(c.idIter, 1)
		client.id = id
		c.config.Handler.OnConnect(client)
		if !client.Stop {
			go client.Run()
		}
	}
}

func (c *Core) deleteClient(client *Client) {
	delete(c.clientPool, client.id)
}
