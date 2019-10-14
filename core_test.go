/*
@Time : 2019-10-09 14:27
@Author : zr
*/
package socket_server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/ZR233/socket_server/handler"
	"log"
	"net/http"
	_ "net/http/pprof"
	"testing"
)

type TestHandler struct {
}

func (t TestHandler) OnConnect(client handler.Client) {
	println("new client")
}

func (t TestHandler) HeaderHandler(headerData []byte, ctx *handler.Context) (bodyLen int, err error) {
	bytesBuffer := bytes.NewBuffer(headerData)
	var j uint32
	err = binary.Read(bytesBuffer, binary.BigEndian, &j)
	bodyLen = int(j)
	fmt.Printf("body len %d", bodyLen)
	return
}
func (t TestHandler) BodyHandler(bodyData []byte, ctx *handler.Context) (err error) {
	println(string(bodyData))
	return
}
func (t TestHandler) OnError(err error, ctx *handler.Context) {
	println(err.Error())
	return
}
func (t TestHandler) HeaderLen() int {
	return 4
}

type logger struct {
}

func (l logger) Warn(msg ...interface{}) {
	for _, v := range msg {
		println(v.(string))
	}

}

func (l logger) Info(msg ...interface{}) {
	for _, v := range msg {
		println(v.(string))
	}
}

func (l logger) Debug(msg ...interface{}) {
	for _, v := range msg {
		println(v.(string))
	}
}

func TestNewCore(t *testing.T) {
	//f, err := os.Create("mem.prof")
	//if err != nil {
	//	return
	//}
	//pprof.WriteHeapProfile(f)
	//defer pprof.StopCPUProfile()

	h := TestHandler{}

	config := Config{
		ListenIP: "0.0.0.0",
		Port:     29999,
		Handler:  h,
	}
	core := NewCore(&config)
	core.SetLogger(logger{})
	go core.Run()

	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
	return
}
