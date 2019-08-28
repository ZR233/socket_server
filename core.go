/*
@Time : 2019-08-26 16:24
@Author : zr
*/
package socket_server

import (
	"fmt"
	"net"
	"sync/atomic"
)

type Config struct {
	ListenIP string
	Port     int
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
	OnConnect  func(client *Client)
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
		client := newClient(&conn, c)
		id := atomic.AddUint32(c.idIter, 1)
		client.id = id
		c.OnConnect(client)
		if !client.Stop {
			go client.Run()
		}
	}
}
