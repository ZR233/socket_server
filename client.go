/*
@Time : 2019-08-28 14:28
@Author : zr
*/
package socket_server

import (
	"errors"
	"net"
)

type Client struct {
	conn     *net.Conn
	core     *Core
	id       uint32
	stopChan chan bool
	Stop     bool
}

func newClient(conn *net.Conn, core *Core) *Client {
	c := &Client{
		conn: conn,
		core: core,
	}
	c.stopChan = make(chan bool, 1)
	c.Stop = false
	go func() {
		c.Stop = <-c.stopChan
	}()

	return c
}

func (c Client) Run() {

}

func (c *Client) Close() error {
	select {
	case c.stopChan <- false:
		return nil
	default:
		return errors.New("closed")
	}
}
