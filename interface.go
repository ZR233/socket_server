/*
@Time : 2019-08-28 14:27
@Author : zr
*/
package socket_server

import "github.com/ZR233/goutils/errors2"

type Command interface {
	Exec() (err error)
}

type Handler interface {
	HeaderHandler(headerData []byte, client *Client) (bodyLen int, err error)
	BodyHandler(bodyData []byte, client *Client) (err error)
	OnConnect(client *Client)
	OnError(stdErr *errors2.StdError, client *Client)
	HeaderLen() int
	OnClose(client *Client)
}
