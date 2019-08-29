/*
@Time : 2019-08-28 14:28
@Author : zr
*/
package socket_server

import (
	"errors"
	"net"
	"sync"
)

type Client struct {
	conn       net.Conn
	core       *Core
	Id         uint32
	writeChan  chan []byte
	headerBuff []byte
	stopChan   chan bool
	Stop       bool
}

func newClient(conn net.Conn, core *Core) *Client {
	c := &Client{
		conn: conn,
		core: core,
	}
	c.stopChan = make(chan bool, 1)
	headerLen := c.core.config.HeaderLen
	c.headerBuff = make([]byte, headerLen)
	c.Stop = false
	go func() {
		c.Stop = <-c.stopChan
	}()

	return c
}

func (c *Client) Run() {
	defer func() {
		close(c.writeChan)
		close(c.stopChan)
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

func (c *Client) onError(err error, ctx *Context) {
	defer func() {
		if err := recover(); err != nil {
			c.core.logger.Warn(err)
		}
	}()

	c.core.config.OnError(err, ctx)
}

func (c *Client) readLoop() {
	ctx := &Context{
		Client: c,
	}

	defer func() {
		if err := recover(); err != nil {
			err_ := err.(error)
			c.onError(err_, ctx)
		}
	}()
	n, err := c.conn.Read(c.headerBuff)
	if err != nil {
		panic(err)
	}

	if n != c.core.config.HeaderLen {
		panic(errors.New("header len error"))
	}

	bodyLen, err := c.core.config.HeaderHandler(c.headerBuff, ctx)

	buff := make([]byte, bodyLen)
	n, err = c.conn.Read(buff)
	if err != nil {
		panic(err)
	}

	if n != bodyLen {
		panic(errors.New("body len error"))
	}

	err = c.core.config.BodyHandler(buff, ctx)
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
		panic(err)
	}
	if n != len(data) {
		panic(errors.New("write len error"))
	}
}

func (c *Client) Write(data []byte) {
	c.writeChan <- data
}

func (c *Client) Close() error {
	select {
	case c.stopChan <- false:
	_:
		c.conn.Close()
		return nil
	default:
		return errors.New("closed")
	}
}
