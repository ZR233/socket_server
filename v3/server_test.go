package v2

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

var maxD time.Duration
var mu sync.Mutex

type TestHandler struct {
}

func (t TestHandler) OnConnect(client *Session) {
	//fmt.Printf("new client: %d\n", client.id)
}

func (t TestHandler) OnClose(client *Session) {
	fmt.Printf("close: %d\n", client.id)
}
func (t TestHandler) HeaderHandler(headerData []byte, client *Session, thisRecv context.Context) (
	bodyLen int, thisRecvUpdated context.Context, err error) {
	bytesBuffer := bytes.NewBuffer(headerData)
	var j uint16
	err = binary.Read(bytesBuffer, binary.BigEndian, &j)
	bodyLen = int(j)

	return
}
func (t TestHandler) BodyHandler(bodyData []byte, client *Session, thisRecv context.Context) (err error) {
	bodyLen := uint16(len(bodyData))

	bytesBuffer := bytes.NewBuffer([]byte{})
	_ = binary.Write(bytesBuffer, binary.BigEndian, bodyLen)

	bytesBuffer.Write(bodyData)

	cmd := client.NewCommandWrite(bytesBuffer.Bytes())
	client.ExecCommands(cmd)

	return
}
func (t TestHandler) OnError(err error, client *Session) {
	fmt.Printf("%d: %s", client.id, err)
	return
}

func (t TestHandler) HeaderLen() int {
	return 2
}
func TestServer_onAccept(t *testing.T) {
	addr := "127.0.0.1:29999"

	h := TestHandler{}

	config := &Options{
		Addr:    addr,
		Handler: h,
		//ReadTimeout: time.Millisecond* 500,
	}
	server := NewServer(config)
	logrus.SetLevel(logrus.DebugLevel)
	go server.Run()

	//go func() {
	//	for i := 0; i < 5000; i++ {
	//
	//		//fmt.Printf("%d\r\n", i)
	//		go func() {
	//			newTestClient(addr)
	//		}()
	//
	//	}
	//}()
	//go func() {
	//	for{
	//		time.Sleep(time.Second*5)
	//		fmt.Printf("%s\n", maxD)
	//	}
	//}()

	log.Fatal(http.ListenAndServe("0.0.0.0:18080", nil))
	return

}

type msg struct {
	Ts   int64
	Data string
}

func setMaxD(d time.Duration) {
	mu.Lock()
	defer mu.Unlock()

	if d > maxD {
		maxD = d
		fmt.Printf("%s\n", maxD)
	}
}

func cConnHandler(c net.Conn) {
	loopCount := 1000

	go func() {

		headBuf := make([]byte, 4)
		for i := 0; i < loopCount; i++ {

			_, err := c.Read(headBuf)
			if err != nil {
				return
			}

			bytesBuffer := bytes.NewBuffer(headBuf)
			var j uint32
			err = binary.Read(bytesBuffer, binary.BigEndian, &j)
			bodyLen := int(j)

			bodyBuf := make([]byte, bodyLen)
			_, err = c.Read(bodyBuf)
			if err != nil {
				return
			}

			m := &msg{}

			err = json.Unmarshal(bodyBuf, m)
			if err != nil {
				panic(err)
			}
			setMaxD(time.Now().Sub(time.Unix(0, m.Ts)))
		}
		println("finish")
		c.Close()
	}()
	for i := 0; i < loopCount; i++ {
		time.Sleep(time.Second)
		msg_ := &msg{}
		msg_.Data = "acvhmnyukjhgnfghmfhgmfghnsfghbsrbrtbawdwadwadawddadwadaawsdascascsxcasdvaergaafefawefadavadsvaebvadsfawefasdfcasdvqasdfasdfsad"
		msg_.Ts = time.Now().UnixNano()

		data, _ := json.Marshal(msg_)
		bodyLen := uint32(len(data))

		bytesBuffer := bytes.NewBuffer([]byte{})
		err := binary.Write(bytesBuffer, binary.BigEndian, bodyLen)
		if err != nil {
			panic(err)
		}
		_, err = c.Write(bytesBuffer.Bytes())
		if err != nil {
			panic(err)
		}
		rand.Seed(time.Now().UnixNano())
		str := hex.EncodeToString(data)
		println(str)
		iter := rand.Intn(int(bodyLen))
		_, err = c.Write(data[:iter])
		if err != nil {
			panic(err)
		}
		_, err = c.Write(data[iter:])
		if err != nil {
			panic(err)
		}
	}
}

type testClient struct {
	conn net.Conn
}

func newTestClient(addr string) *testClient {
	t := &testClient{}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		fmt.Println("Resolve TCPAddr error", err)
	}
	conn, err := net.DialTCP("tcp4", nil, tcpAddr)
	if err != nil {
		panic(err)
	}
	t.conn = conn
	time.Sleep(time.Second * 3)
	cConnHandler(conn)
	return t
}
