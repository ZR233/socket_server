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
	state           ClientState
	stateMu         *sync.Mutex
	ctx             context.Context
	goroutineCancel context.CancelFunc
	tcpDeadLine     time.Duration
	readByte        []byte
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
	c.ctx = ctx
	c.goroutineCancel = cancel
	c.cmdChan = make(chan Command, 10)
	headerLen := c.core.config.Handler.HeaderLen()
	c.headerBuff = make([]byte, headerLen)
	c.readByte = []byte{0}
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
	defer func() {
		recover()
	}()
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
	c.headerBuff, err = ReadFrom(c.conn, int64(c.core.config.Handler.HeaderLen()), c.tcpDeadLine)
	return
}

func (c *Client) readLoopGetBodyLen() (bodyLen int, err error) {
	bodyLen, err = c.core.config.Handler.HeaderHandler(c.headerBuff, c)
	if err != nil {
		err = errors.U.NewStdError(errors.Header, err.Error())
		return
	}

	if bodyLen > 1000*10 {
		err = errors.U.NewStdError(errors.Header, "body too long")
		return
	}

	return
}
func newDataBuff(len int64) []byte {
	return make([]byte, len)
}

func (c *Client) connRead(data []byte) (n int, err error) {
	n, err = c.conn.Read(data)
	return
}

func ReadFrom(conn net.Conn, wantLen int64, timeout time.Duration) (data []byte, err error) {
	if wantLen == 0 {
		return
	}
	expireAt := time.Now().Add(timeout)
	readLen := int64(0)
	err = conn.SetReadDeadline(expireAt)
	if err != nil {
		err = errors.U.FromError(err, errors.Socket)
		return
	}

	for {
		if expireAt.Sub(time.Now()) <= 0 {
			err = errors.U.NewStdError(errors.Socket, "read time out")
			return
		}
		if readLen == wantLen {
			break
		}
		dataBatch := newDataBuff(wantLen - readLen)

		n := 0
		n, err = conn.Read(dataBatch)
		if err != nil {
			return
		}
		data = append(data, dataBatch[:n]...)
		readLen += int64(n)
	}

	return
}

func copyData(data []byte) (data2 []byte) {
	dataLen := len(data)
	data2 = make([]byte, dataLen)
	copy(data2, data)
	return
}

func (c *Client) readLoopDealBody(data []byte) (err error) {
	err = c.core.config.Handler.BodyHandler(copyData(data), c)
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
		if c.Stopped() {
			err = nil
			return
		}
		return
	}

	bodyLen, err := c.readLoopGetBodyLen()
	if err != nil {
		return
	}
	data, err := ReadFrom(c.conn, int64(bodyLen), time.Second*2)
	if err != nil {
		if c.Stopped() {
			err = nil
			return
		}
		return
	}
	err = c.readLoopDealBody(data)
	data = nil
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
	c.core.config.Handler.OnClose(c)
	_ = c.conn.Close()
	return nil
}
