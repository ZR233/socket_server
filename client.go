/*
@Time : 2019-08-28 14:28
@Author : zr
*/
package socket_server

import (
	"context"
	"fmt"
	"github.com/ZR233/socket_server/errors"
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
	conn            net.Conn
	core            *Core
	id              uint32
	cmdChan         chan Command
	headerBuff      []byte
	clientCtx       *Context
	Fields          interface{}
	state           ClientState
	stateMu         *sync.Mutex
	ctx             context.Context
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

func (c *Client) GetCxt() *Context {
	return c.clientCtx
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
	c.ctx = ctx
	c.clientCtx = &Context{}
	c.goroutineCancel = cancel
	c.cmdChan = make(chan Command, 10)
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
		close(c.cmdChan)
		c.core = nil
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

func (c *Client) ExecCommands(commands ...Command) {
	for _, command := range commands {
		c.cmdChan <- command
	}
}

func (c *Client) onError(err error) {
	defer func() {
		if e := recover(); e != nil {
			logrus.Error(fmt.Sprintf("(%d)OnError err:%s", c.id, err))
		}
	}()
	stdErr := errors.U.FromError(err, errors.Unknown)
	c.core.config.Handler.OnError(stdErr, c)
}
func (c *Client) Stopped() bool {
	return c.state >= ClientStateStopping
}

func (c *Client) readLoopDefer(err *error) {
	if e := recover(); e != nil {
		msg := fmt.Sprintf("(%d)error\r\n%s", c.id, e)
		*err = errors.U.NewStdError(errors.Unknown, msg)
	}

	if *err != nil {
		c.onError(*err)
		_ = c.Close()
	}
}
func (c *Client) readLoopGetHeadData() (err error) {
	err = c.conn.SetReadDeadline(time.Now().Add(c.tcpDeadLine))
	if err != nil {
		err = errors.U.FromError(err, errors.Socket)
		return
	}
	logrus.Debug(fmt.Sprintf("(%d)", c.id), "read header, time out:", c.tcpDeadLine.String())
	n, err := c.conn.Read(c.headerBuff)
	if err != nil {
		if c.Stopped() {
			err = nil
		} else {
			err = errors.U.FromError(err, errors.Socket)
		}
		return
	}

	if n != c.core.config.Handler.HeaderLen() {
		err = errors.U.NewStdError(errors.Header, "header len error")
		return
	}
	return
}

func (c *Client) readLoopGetBodyLen() (bodyLen int, err error) {
	bodyLen, err = c.core.config.Handler.HeaderHandler(c.headerBuff, c)
	if err != nil {
		err = errors.U.NewStdError(errors.Header, err.Error())
		return
	}
	return
}
func newDateBuff(len int) []byte {
	return make([]byte, len)
}

func (c *Client) connRead(data []byte) (n int, err error) {
	n, err = c.conn.Read(data)
	return
}

func (c *Client) readLoopGetBodyData(bodyLen int) (data []byte, err error) {
	if bodyLen == 0 {
		return
	}
	expireAt := time.Now().Add(time.Second * 3)
	data = newDateBuff(bodyLen)

	readLen := 0
	logrus.Debug(fmt.Sprintf("(%d)", c.id), "read body")

	for {
		if expireAt.Sub(time.Now()) <= 0 {
			err = errors.U.NewStdError(errors.Socket, "read time out")
			return
		}
		if readLen == bodyLen {
			break
		}

		err = c.conn.SetReadDeadline(expireAt)
		if err != nil {
			if c.Stopped() {
				err = nil
			} else {
				err = errors.U.FromError(err, errors.Socket)
			}
			return
		}
		n := 0
		n, err = c.connRead(data[readLen:])
		if err != nil {
			if c.Stopped() {
				err = nil
			} else {
				err = errors.U.FromError(err, errors.Socket)
			}
			return
		}

		readLen += n
	}

	if readLen != bodyLen {
		err = errors.U.NewStdError(errors.Socket, "body len error")
		return
	}
	return
}
func (c *Client) readLoopDealBody(data []byte) (err error) {
	err = c.core.config.Handler.BodyHandler(data, c)
	if err != nil {
		err = errors.U.NewStdError(errors.Body, err.Error())
	}
	return
}

func (c *Client) readLoop() {
	var err error
	defer c.readLoopDefer(&err)

	err = c.readLoopGetHeadData()
	if err != nil {
		return
	}

	bodyLen, err := c.readLoopGetBodyLen()
	if err != nil {
		return
	}
	data, err := c.readLoopGetBodyData(bodyLen)
	if err != nil {
		return
	}
	err = c.readLoopDealBody(data)
}

func (c *Client) execCommandLoop() {
	defer func() {
		if e := recover(); e != nil {
			logrus.Error(fmt.Sprintf("(%d)%s\r\n%s", c.id, e, debug.Stack()))
		}
	}()

	select {
	case cmd, ok := <-c.cmdChan:
		if ok {
			if err := cmd.Exec(); err != nil {
				logrus.Error(fmt.Sprintf("(%d)%s", c.id, err))
				c.onError(err)
			}
		} else {
			return
		}
	case <-c.ctx.Done():
		return
	}
}
func (c *Client) writeDefer(err *error) {
	if *err != nil {
		c.onError(*err)
		_ = c.Close()
	}
}

func (c *Client) Write(data []byte) (err error) {
	defer c.writeDefer(&err)

	defer func() {
		if e := recover(); e != nil {
			err = errors.U.NewStdError(errors.Unknown, fmt.Sprintf("write err\r\n%s", e))
		}
	}()
	logrus.Debug(fmt.Sprintf("(%d)send len(%d)", c.id, len(data)))

	err = c.conn.SetWriteDeadline(time.Now().Add(c.tcpDeadLine))
	if err != nil {
		if c.Stopped() {
			err = nil
		} else {
			err = errors.U.FromError(err, errors.Socket)
		}
		return
	}

	n, err := c.conn.Write(data)
	if err != nil {
		if c.Stopped() {
			err = nil
		} else {
			err = errors.U.FromError(err, errors.Socket)
		}
		return
	}
	if n != len(data) {
		err = errors.U.NewStdError(errors.Socket, "write len error")
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
