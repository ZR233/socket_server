/*
@Time : 2019-08-28 14:28
@Author : zr
*/
package socket_server

import (
	"errors"
	"fmt"
	"github.com/ZR233/socket_server/handler"
	"net"
	"strconv"
	"sync"
)

type ClientState int

const (
	_ ClientState = iota
	ClientStateRunning
	ClientStateStopping
)

type Client struct {
	conn       net.Conn
	core       *Core
	id         uint32
	writeChan  chan []byte
	headerBuff []byte
	stopChan   chan bool
	ctx        *handler.Context
	Fields     interface{}
	state      ClientState
	logger     Logger
}

func (c *Client) setState(state ClientState) {
	c.state = state
}

func (c *Client) GetState() (state ClientState) {
	return c.state
}

func newClient(conn net.Conn, core *Core, logger Logger) *Client {
	c := &Client{
		conn:   conn,
		core:   core,
		state:  ClientStateRunning,
		logger: logger,
	}
	c.ctx = &handler.Context{Client: c}
	c.stopChan = make(chan bool, 1)
	c.writeChan = make(chan []byte, 10)
	headerLen := c.core.config.Handler.HeaderLen()
	c.headerBuff = make([]byte, headerLen)

	return c
}

func (c *Client) Id() uint32 {
	return c.id
}
func (c *Client) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}
func (c *Client) Run() {
	defer func() {
		c.core.deleteClient(c)
		close(c.writeChan)
		close(c.stopChan)
		c.core = nil
		c.ctx.Client = nil
		c.ctx = nil
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			if c.state == ClientStateRunning {
				c.readLoop()
			} else {
				break
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			if c.state == ClientStateRunning {
				c.writeLoop()
			} else {
				break
			}
		}
	}()

	wg.Wait()

}

func (c *Client) onError(err error, ctx *handler.Context) {
	defer func() {
		if err := recover(); err != nil {
			c.core.logger.Warn(err)
		}
	}()
	c.core.config.Handler.OnError(err, ctx)
}
func (c *Client) Stopped() bool {
	return c.state == ClientStateStopping
}
func (c *Client) readLoop() {
	ctx := &handler.Context{
		Client: c,
	}

	defer func() {
		if err := recover(); err != nil {
			err_ := err.(error)
			c.onError(err_, ctx)
		_:
			c.Close()
		}

		c.ctx.Keys = nil

	}()
	n, err := c.conn.Read(c.headerBuff)
	if err != nil {
	_:
		c.Close()
		return
	}

	if n != c.core.config.Handler.HeaderLen() {
		panic(errors.New("header len error"))
	}

	bodyLen, err := c.core.config.Handler.HeaderHandler(c.headerBuff, ctx)
	if err != nil {
		panic(err)
	}

	buff := make([]byte, bodyLen)
	n, err = c.conn.Read(buff)
	if err != nil {
		panic(err)
	}

	if n != bodyLen {
		panic(errors.New("body len error"))
	}

	err = c.core.config.Handler.BodyHandler(buff, ctx)
	if err != nil {
		panic(err)
	}
}

func (c *Client) writeLoop() {
	defer func() {
		if err := recover(); err != nil {
			err_ := err.(error)
			c.onError(err_, nil)
		_:
			c.Close()
		}

	}()
	c.logger.Debug("wait for send data")

	data := <-c.writeChan
	c.logger.Debug("got send data")
	n, err := c.conn.Write(data)
	if err != nil {
	_:
		c.Close()
		return
	}
	if n != len(data) {
		panic(errors.New("write len error"))
	}
	c.logger.Debug("send success:", strconv.Itoa(n))
}

func (c *Client) Write(data []byte) {
	c.logger.Debug(fmt.Sprintf("send len(%d)", len(data)))
	c.writeChan <- data
}

func (c *Client) Close() error {
	c.logger.Debug(fmt.Sprintf("close(%d)", c.id))
	c.setState(ClientStateStopping)
	return c.conn.Close()
}

func (c *Client) SetFields(fields interface{}) {
	c.Fields = fields
}
func (c *Client) GetFields() interface{} {
	return c.Fields
}
