/*
@Time : 2019-10-09 14:44
@Author : zr
*/
package socket_server

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestClient_Close(t *testing.T) {
	for i := 0; i < 20000; i++ {
		server := "localhost:29999"
		tcpAddr, err := net.ResolveTCPAddr("tcp4", server)

		if err != nil {
			panic(err)
		}

		//建立服务器连接
		conn, err := net.DialTCP("tcp", nil, tcpAddr)

		if err != nil {
			panic(err)
		}

		fmt.Println("connection success")
		sender(conn)
		fmt.Println("send over")
		conn.Close()
	}

}

func sender(conn *net.TCPConn) {
	bodyData := "hello"

	bodyLen := uint32(len(bodyData))
	//bodyLen :=uint32(50000)
	header := make([]byte, 4)

	binary.BigEndian.PutUint32(header, bodyLen)

	n, err := conn.Write(header) //给服务器发信息
	if err != nil {
		panic(err)
	}
	fmt.Printf("send %d", n)

	n, err = conn.Write([]byte(bodyData)) //给服务器发信息
	if err != nil {
		panic(err)
	}
	fmt.Printf("send %d", n)
}

func TestClient_Send(t *testing.T) {
	fmt.Println("client launch")
	serverAddr := "localhost:8888"
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		fmt.Println("Resolve TCPAddr error", err)
	}
	conn, err := net.DialTCP("tcp4", nil, tcpAddr)
	defer conn.Close()
	if err != nil {
		fmt.Println("connect server error", err)
	}

	conn.Write([]byte("hello"))
	time.Sleep(2 * time.Second) //等两秒钟，不然还没接收数据，程序就结束了。

	conn.Write([]byte("world"))
}
func TestClient_Read(t *testing.T) {
	fmt.Println("hello world")

	lner, err := net.Listen("tcp", "localhost:8888")
	if err != nil {
		fmt.Println("listener creat error", err)
	}
	fmt.Println("waiting for client")
	for {
		conn, err := lner.Accept()
		if err != nil {
			fmt.Println("accept error", err)
		}

		conn.SetDeadline(time.Now().Add(time.Second * 50))
		buf := [10]byte{}

		for {
			_, err := conn.Read(buf[:])
			if err != nil {
				return
			}
			println(string(buf[:]))
		}

	}
}
