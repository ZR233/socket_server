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

//type DefaultHandler struct {
//}
//
//func (d DefaultHandler) HeaderHandler(headerData []byte, client *Client) (bodyLen int, err error) {
//	panic("implement me")
//}
//
//func (d DefaultHandler) BodyHandler(bodyData []byte, client *Client) (err error) {
//	logrus.Debugf()
//	return nil
//}
//
//func (d DefaultHandler) OnConnect(client *Client) {
//	logrus.Debug(fmt.Sprintf("[%d]connected", client.id))
//}
//
//func (d DefaultHandler) OnError(err error, client *Client) {
//	logrus.Error(err)
//}
//
//func (d DefaultHandler) HeaderLen() int {
//	return 4
//}
