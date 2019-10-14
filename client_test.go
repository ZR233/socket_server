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
