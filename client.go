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
	"github.com/sirupsen/logrus"
	"net"
	"runtime/debug"
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

type CommandWrite struct {
	client *Client
	data   []byte
}

func (c *CommandWrite) Exec() (err error) {
	return c.client.Write(c.data)
}

type CommandClose struct {
	client *Client
}

func (c *CommandClose) Exec() (err error) {
	return c.client.Close()
}

type Client struct {
	conn net.Conn
	core *Core
	id   uint32
	//writeChan       chan []byte
	cmdChan         chan handler.Command
	headerBuff      []byte
	ctx             *handler.Context
	Fields          interface{}
	state           ClientState
	stateMu         *sync.Mutex
	goroutineCtx    context.Context
	goroutineCancel context.CancelFunc
	tcpDeadLine     time.Duration
}

func (c *Client) NewCommandWrite(data []byte) *CommandWrite {
	return &CommandWrite{
		client: c,
		data:   data,
	}
}
func (c *Client) NewCommandClose() *CommandClose {
	return &CommandClose{
		client: c,
	}
}

func (c *Client) setState(state ClientState) {
	c.state = state
}

func (c *Client) GetState() (state ClientState) {
	return c.state
}

func newClient(conn net.Conn, id uint32, core *Core, tcpDeadLine time.Duration) *Client {
	c := &Client{
		id:          id,
		conn:        conn,
		core:        core,
		state:       ClientStateRunning,
		stateMu:     &sync.Mutex{},
		tcpDeadLine: tcpDeadLine,
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.goroutineCtx = ctx
	c.goroutineCancel = cancel

	c.ctx = &handler.Context{Client: c}
	//c.writeChan = make(chan []byte, 10)
	c.cmdChan = make(chan handler.Command, 10)
	headerLen := c.core.config.Handler.HeaderLen()
	c.headerBuff = make([]byte, headerLen)
	logrus.Debug(fmt.Sprintf("(%d)create client", id))
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
		logrus.Debug(fmt.Sprintf("(%d)delete", c.id))
		c.core.deleteClient(c)
		//close(c.writeChan)
		close(c.cmdChan)
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
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			if c.state == ClientStateRunning {
				c.execCommandLoop()
			} else {
				return
			}
		}
	}()

	wg.Wait()

}

func (c *Client) ExecCommands(commands ...handler.Command) {
	for _, command := range commands {
		c.cmdChan <- command
	}
}

func (c *Client) onError(err interface{}, ctx *handler.Context) {
	defer func() {
		if err := recover(); err != nil {
			logrus.Warn(fmt.Sprintf("(%d)err:%s", c.id, err))
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
	logrus.Debug(fmt.Sprintf("(%d)", c.id), "read header, time out:", c.tcpDeadLine.String())
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
func newDateBuff(len int) []byte {
	return make([]byte, len)
}

func (c *Client) connRead(data []byte) (n int) {
	n, err := c.conn.Read(data)
	if err != nil {
		panic(err)
	}
	return
}

func (c *Client) readLoopGetBodyData(bodyLen int) (data []byte) {
	if bodyLen == 0 {
		return
	}
	expireAt := time.Now().Add(time.Second * 3)
	data = newDateBuff(bodyLen)

	readLen := 0
	logrus.Debug(fmt.Sprintf("(%d)", c.id), "read body")

	for {
		if expireAt.Sub(time.Now()) <= 0 {
			panic(errors.New("read time out"))
		}
		if readLen == bodyLen {
			break
		}

		err := c.conn.SetReadDeadline(expireAt)
		if err != nil {
			panic(err)
		}
		n := c.connRead(data[readLen:])

		readLen += n
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

func (c *Client) execCommandLoop() {
	defer func() {
		if e := recover(); e != nil {
			logrus.Warn(fmt.Sprintf("(%d)%s\r\n%s", c.id, e, debug.Stack()))
		}
	}()

	select {
	case cmd, ok := <-c.cmdChan:
		if ok {
			if err := cmd.Exec(); err != nil {
				logrus.Warn(fmt.Sprintf("(%d)%s", c.id, err))
				c.onError(err, c.ctx)
			}
		} else {
			return
		}
	case <-c.goroutineCtx.Done():
		return
	}
}

func (c *Client) Write(data []byte) (err error) {
	defer func() {
		if err != nil {
			_ = c.Close()
		}
	}()

	defer func() {
		if e := recover(); e != nil {
			err = errors.New(fmt.Sprintf("%s\r\n%s", e, debug.Stack()))
		}
	}()
	logrus.Debug(fmt.Sprintf("(%d)send len(%d)", c.id, len(data)))

	err = c.conn.SetWriteDeadline(time.Now().Add(c.tcpDeadLine))
	if err != nil {
		return
	}

	n, err := c.conn.Write(data)
	if err != nil {
		return
	}
	if n != len(data) {
		err = errors.New("write len error")
		return
	}
	logrus.Debug(fmt.Sprintf("(%d)", c.id), "send success:", strconv.Itoa(n))
	return
}

func (c *Client) Close() error {
	logrus.Debug(fmt.Sprintf("(%d)close signal", c.id))
	c.stateMu.Lock()
	if c.state < ClientStateStopping {
		c.setState(ClientStateStopping)
		c.stateMu.Unlock()
		c.goroutineCancel()
	} else {
		c.stateMu.Unlock()
	}
	_ = c.conn.Close()
	return nil
}

func (c *Client) SetFields(fields interface{}) {
	c.Fields = fields
}
func (c *Client) GetFields() interface{} {
	return c.Fields
}
