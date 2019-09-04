/*
@Time : 2019-08-28 14:28
@Author : zr
*/
package socket_server

import (
	"errors"
	"github.com/ZR233/socket_server/handler"
	"net"
	"sync"
)

type Client struct {
	conn       net.Conn
	core       *Core
	id         uint32
	writeChan  chan []byte
	headerBuff []byte
	stopChan   chan bool
	Stop       bool
	ctx        *handler.Context
	Fields     interface{}
}

func newClient(conn net.Conn, core *Core) *Client {
	c := &Client{
		conn: conn,
		core: core,
	}
	c.ctx = &handler.Context{Client: c}
	c.stopChan = make(chan bool, 1)
	c.writeChan = make(chan []byte, 10)
	headerLen := c.core.config.Handler.HeaderLen()
	c.headerBuff = make([]byte, headerLen)
	c.Stop = false
	go func() {
		c.Stop = <-c.stopChan
	}()

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
			if !c.Stop {
				c.readLoop()
			} else {
				break
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			if !c.Stop {
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
	return c.Stop
}
func (c *Client) readLoop() {
	ctx := &handler.Context{
		Client: c,
	}

	defer func() {
		if err := recover(); err != nil {
			err_ := err.(error)
			c.onError(err_, ctx)
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
		}
	}()

	data := <-c.writeChan

	n, err := c.conn.Write(data)
	if err != nil {
	_:
		c.Close()
		return
	}
	if n != len(data) {
		panic(errors.New("write len error"))
	}
}

func (c *Client) Write(data []byte) {
	c.writeChan <- data
}

func (c *Client) Close() error {

	c.core.deleteClient(c)
	select {
	case c.stopChan <- true:
	_:
		c.conn.Close()
		return nil
	default:
		return errors.New("closed")
	}
}

func (c *Client) SetFields(fields interface{}) {
	c.Fields = fields
}
func (c *Client) GetFields() interface{} {
	return c.Fields
}
