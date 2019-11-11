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
	"github.com/sirupsen/logrus"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"testing"
	"time"
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
	fmt.Printf("body handler, len %d", len(bodyData))
	return
}
func (t TestHandler) OnError(err error, ctx *handler.Context) {
	println(err.Error())
	return
}
func (t TestHandler) HeaderLen() int {
	return 4
}

func cConnHandler(c net.Conn) {
	readBuf := make([]byte, 1)
	go func() {
		for {
			_, err := c.Read(readBuf)
			if err != nil {
				return
			}
		}
	}()

	for i := 0; i < 1000; i++ {
		bodyLen := uint32(2200)
		bodyData := make([]byte, bodyLen)

		bytesBuffer := bytes.NewBuffer([]byte{})
		err := binary.Write(bytesBuffer, binary.BigEndian, bodyLen)
		if err != nil {
			panic(err)
		}
		fmt.Printf("body len %d", bodyLen)
		_, _ = c.Write(bytesBuffer.Bytes())
		rand.Seed(time.Now().UnixNano())

		iter := rand.Intn(int(bodyLen))
		_, _ = c.Write(bodyData[:iter])
		_, _ = c.Write(bodyData[iter:])
	}

	c.Close()
}

type testClient struct {
	conn net.Conn
}

func newTestClient(addr string) *testClient {
	t := &testClient{}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	t.conn = conn
	cConnHandler(conn)
	return t
}

func TestNewCore(t *testing.T) {
	addr := "127.0.0.1:29999"

	h := TestHandler{}

	config := Config{
		ListenIP: "0.0.0.0",
		Port:     29999,
		Handler:  h,
	}
	core := NewCore(&config)
	logrus.SetLevel(logrus.DebugLevel)
	go core.Run()

	go func() {
		for i := 0; i < 5000; i++ {
			fmt.Printf("%d\r\n", i)
			newTestClient(addr)
		}
	}()

	log.Fatal(http.ListenAndServe("0.0.0.0:18080", nil))
	return
}
