/*
@Time : 2019-08-28 14:27
@Author : zr
*/
package handler

import "github.com/ZR233/socket_server"

type Handler interface {
	HeaderHandler(headerData []byte, ctx *Context) (bodyLen int, err error)
	BodyHandler(bodyData []byte, ctx *Context) (err error)
	OnConnect(client *socket_server.Client)
	OnError(err error, ctx *Context)
}
