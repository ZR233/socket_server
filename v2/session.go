package v2

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"
)

type Session struct {
	id     uint32
	server *Server
	net.Conn
	cmdChan chan CmdInterface

	ctxCmd     context.Context
	cancelCmd  context.CancelFunc
	ctxRead    context.Context
	cancelRead context.CancelFunc

	readBuff []byte
	Fields   interface{}

	closeChan chan bool
}

func (s *Session) close() {
	s.server.options.Handler.OnClose(s)
	close(s.closeChan)
	s.server.deleteSession(s.id)
	s.cancelCmd()
	s.cancelRead()
	_ = s.Conn.Close()
	close(s.cmdChan)
	return
}

func (s *Session) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("already closed")
		}
	}()
	s.closeChan <- true
	return nil
}

func (s *Session) NewCmdClose() *CmdClose {
	return &CmdClose{
		session: s,
	}
}

func newSession(id uint32, server *Server, conn net.Conn) (s *Session) {
	s = &Session{
		id:        id,
		server:    server,
		Conn:      conn,
		cmdChan:   make(chan CmdInterface, 10),
		closeChan: make(chan bool),
	}
	s.readBuff = make([]byte, s.server.options.BuffLen)
	s.ctxCmd, s.cancelCmd = context.WithCancel(s.server.ctxCmd)
	s.ctxRead, s.cancelRead = context.WithCancel(s.server.ctxCmd)
	go func() {
		<-s.closeChan
		s.close()
	}()

	s.server.options.Handler.OnConnect(s)
	go s.runReadLoop()
	go s.runCmdLoop()

	return
}

func (s *Session) runReadLoop() {
	for {
		if s.ctxRead.Err() != nil {
			return
		}
		s.readLoop()
	}
}

func (s *Session) runCmdLoop() {
	for {
		select {
		case <-s.ctxCmd.Done():
			return
		case cmd, ok := <-s.cmdChan:
			if ok {
				cmd.Exec()
			}
		}
	}
}

func (s *Session) readLoop() {
	defer func() {
		if p := recover(); p != nil {
			s.handleError(fmt.Errorf("%s", p))
		}
	}()

	handler := s.server.options.Handler
	headerData := s.read(handler.HeaderLen())
	bodyLen, _ := handler.HeaderHandler(headerData, s)
	bodyData := s.read(bodyLen)
	_ = handler.BodyHandler(bodyData, s)
	return
}

func (s *Session) read(wantLen int) (data []byte) {
	deadline := time.Now().Add(s.server.options.ReadTimeout)

	if s.server.options.ReadTimeout == 0 {
		deadline = time.Time{}
	}

	err := s.SetReadDeadline(deadline)
	if err != nil {
		s.handleError(err)
		_ = s.Close()
		return
	}
	buf := bytes.NewBuffer(data)
	n := 0

	for {
		if buf.Len() == wantLen {
			break
		}
		buffLen := wantLen - buf.Len()
		if buffLen > s.server.options.BuffLen {
			buffLen = s.server.options.BuffLen
		}

		dataBatch := s.readBuff[:buffLen]

		n, err = s.Read(dataBatch)
		if err != nil {
			s.handleError(err)
			_ = s.Close()
			return
		}
		buf.Write(dataBatch[:n])
	}

	return buf.Bytes()
}

func (s *Session) handleError(err error) {
	if s.ctxCmd.Err() == nil {
		s.server.handleError(err, s)
	}
}

func (s *Session) NewCommandWrite(data []byte) *CmdWrite {
	return &CmdWrite{
		session: s,
		data:    data,
	}
}

func (s *Session) ExecCommands(commands ...CmdInterface) {
	defer func() {
		recover()
	}()
	for _, command := range commands {
		s.cmdChan <- command
	}
}

func (s *Session) Id() uint32 {
	return s.id
}
func (s *Session) Stopped() bool {
	return s.ctxCmd.Err() != nil
}
