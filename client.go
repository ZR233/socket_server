/*
@Time : 2019-08-28 14:28
@Author : zr
*/
package socket_server

import (
	"context"
	"errors"
	"fmt"
	"github.com/ZR233/socket_server/handler"
	"net"
	"strconv"
	"sync"
	"time"
)

type ClientState int

const (
	_ ClientState = iota
	ClientStateRunning
	ClientStateStopping
	ClientStateStopped
)

type Client struct {
	conn            net.Conn
	core            *Core
	id              uint32
	writeChan       chan []byte
	headerBuff      []byte
	stopChan        chan bool
	ctx             *handler.Context
	Fields          interface{}
	state           ClientState
	stateMu         *sync.Mutex
	goroutineCtx    context.Context
	goroutineCancel context.CancelFunc
	logger          Logger
	tcpDeadLine     time.Duration
}

func (c *Client) setState(state ClientState) {
	c.state = state
}

func (c *Client) GetState() (state ClientState) {
	return c.state
}

func newClient(conn net.Conn, id uint32, core *Core, logger Logger, tcpDeadLine time.Duration) *Client {
	c := &Client{
		id:          id,
		conn:        conn,
		core:        core,
		state:       ClientStateRunning,
		stateMu:     &sync.Mutex{},
		logger:      logger,
		tcpDeadLine: tcpDeadLine,
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.goroutineCtx = ctx
	c.goroutineCancel = cancel

	c.ctx = &handler.Context{Client: c}
	c.stopChan = make(chan bool, 1)
	c.writeChan = make(chan []byte, 10)
	headerLen := c.core.config.Handler.HeaderLen()
	c.headerBuff = make([]byte, headerLen)
	c.logger.Debug(fmt.Sprintf("(%d)create client", id))
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
		c.logger.Debug(fmt.Sprintf("(%d)delete", c.id))
		c.core.deleteClient(c)
		close(c.writeChan)
		close(c.stopChan)
		c.core = nil
		c.ctx.Client = nil
		c.ctx = nil
		c.state = ClientStateStopped
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

func (c *Client) onError(err interface{}, ctx *handler.Context) {
	defer func() {
		if err := recover(); err != nil {
			c.core.logger.Warn(fmt.Sprintf("(%d)err:%s", c.id, err))
		}
	}()
	err_ := err.(error)
	c.core.config.Handler.OnError(err_, ctx)
}
func (c *Client) Stopped() bool {
	return c.state >= ClientStateStopping
}

func (c *Client) readLoopDefer(ctx *handler.Context) {
	if err := recover(); err != nil {
		c.onError(err, ctx)
	_:
		c.Close()
	}

	c.ctx.Keys = nil
	c.ctx.Client = nil
}
func (c *Client) readLoopGetHeadData() {
	err := c.conn.SetReadDeadline(time.Now().Add(c.tcpDeadLine))
	if err != nil {
		panic(err)
	}
	c.logger.Debug(fmt.Sprintf("(%d)", c.id), "read header, time out:", c.tcpDeadLine.String())
	n, err := c.conn.Read(c.headerBuff)
	if err != nil {
		panic(err)
	}

	if n != c.core.config.Handler.HeaderLen() {
		panic(errors.New("header len error"))
	}
	return
}

func (c *Client) readLoopGetBodyLen(ctx *handler.Context) (bodyLen int) {
	bodyLen, err := c.core.config.Handler.HeaderHandler(c.headerBuff, ctx)
	if err != nil {
		panic(err)
	}
	return
}

func (c *Client) readLoopGetBodyData(bodyLen int) (data []byte) {

	readLen := 0
	breakFlag := false

	timer := time.NewTimer(c.tcpDeadLine)
	go func() {
		<-timer.C
		breakFlag = true
	}()

	for {
		if breakFlag {
			break
		}
		if readLen == bodyLen {
			timer.Reset(0)
			break
		}

		var dataPart []byte
		err := c.conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
		if err != nil {
			panic(err)
		}
		c.logger.Debug(fmt.Sprintf("(%d)", c.id), "read body")
		n, err := c.conn.Read(dataPart)
		if err != nil {
			panic(err)
		}
		readLen += n
		data = append(data, dataPart...)
	}

	if readLen != bodyLen {
		panic(errors.New("body len error"))
	}
	return
}
func (c *Client) readLoopDealBody(data []byte, ctx *handler.Context) {
	err := c.core.config.Handler.BodyHandler(data, ctx)
	if err != nil {
		panic(err)
	}
	return
}

func (c *Client) readLoop() {
	ctx := &handler.Context{
		Client: c,
	}
	defer c.readLoopDefer(ctx)

	c.readLoopGetHeadData()
	bodyLen := c.readLoopGetBodyLen(ctx)
	data := c.readLoopGetBodyData(bodyLen)
	c.readLoopDealBody(data, ctx)
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
	c.logger.Debug(fmt.Sprintf("(%d)", c.id), "wait for send data")

	var data []byte
	select {
	case data = <-c.writeChan:
	case <-c.goroutineCtx.Done():
	_:
		c.Close()
		return
	}

	c.logger.Debug(fmt.Sprintf("(%d)", c.id), "got send data")
	err := c.conn.SetWriteDeadline(time.Now().Add(c.tcpDeadLine))
	if err != nil {
		panic(err)
	}

	n, err := c.conn.Write(data)
	if err != nil {
		panic(err)
	}
	if n != len(data) {
		panic(errors.New("write len error"))
	}
	c.logger.Debug(fmt.Sprintf("(%d)", c.id), "send success:", strconv.Itoa(n))
}

func (c *Client) Write(data []byte) {
	c.logger.Debug(fmt.Sprintf("(%d)send len(%d)", c.id, len(data)))
	c.writeChan <- data
}

func (c *Client) Close() error {
	c.logger.Debug(fmt.Sprintf("(%d)close signal", c.id))
	c.stateMu.Lock()
	if c.state == ClientStateRunning {
		c.setState(ClientStateStopping)
		c.stateMu.Unlock()
		c.goroutineCancel()
	} else {
		c.stateMu.Unlock()
	}

	return c.conn.Close()
}

func (c *Client) SetFields(fields interface{}) {
	c.Fields = fields
}
func (c *Client) GetFields() interface{} {
	return c.Fields
}
