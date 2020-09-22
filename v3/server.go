package v2

import (
	"context"
	"net"
	"sync"
	"time"
)

type Handler interface {
	HeaderHandler(headerData []byte, client *Session, thisRecv context.Context) (bodyLen int, thisRecvUpdated context.Context, err error)
	BodyHandler(bodyData []byte, client *Session, thisRecv context.Context) (err error)
	OnConnect(client *Session)
	OnError(err error, client *Session)
	HeaderLen() int
	OnClose(client *Session)
}

type Options struct {
	Addr        string
	Handler     Handler
	BuffLen     int
	ReadTimeout time.Duration
}

type Server struct {
	ctxCmd        context.Context
	cancelCmd     context.CancelFunc
	ctxAccept     context.Context
	cancelAccept  context.CancelFunc
	options       *Options
	cmdChan       chan CmdInterface
	idIter        uint32
	sessionPool   map[uint32]*Session
	sessionPoolMu sync.Mutex
	listener      net.Listener
}

func NewServer(options *Options) *Server {
	s := &Server{
		cmdChan:     make(chan CmdInterface, 10),
		options:     options,
		sessionPool: make(map[uint32]*Session),
	}
	if s.options.BuffLen == 0 {
		s.options.BuffLen = 1024
	}
	s.ctxCmd, s.cancelCmd = context.WithCancel(context.Background())
	s.ctxAccept, s.cancelAccept = context.WithCancel(s.ctxCmd)

	return s
}

func (s *Server) Run() (err error) {
	s.listener, err = net.Listen("tcp", s.options.Addr)
	if err != nil {
		return err
	}

	for {
		s.onAccept()
	}
}

func (s *Server) getSessionId() (id uint32) {

	ok := false
	for {
		s.idIter++
		if _, ok = s.sessionPool[s.idIter]; !ok {
			id = s.idIter
			return
		}
	}
}
func (s *Server) addSession(conn net.Conn) (session *Session) {
	s.sessionPoolMu.Lock()
	defer s.sessionPoolMu.Unlock()

	id := s.getSessionId()
	session = newSession(id, s, conn)
	s.sessionPool[id] = session
	return
}
func (s *Server) deleteSession(id uint32) {
	s.sessionPoolMu.Lock()
	defer s.sessionPoolMu.Unlock()
	delete(s.sessionPool, id)
	return
}
func (s *Server) GetSession(id uint32) (session *Session) {
	s.sessionPoolMu.Lock()
	defer s.sessionPoolMu.Unlock()
	ok := false
	if session, ok = s.sessionPool[id]; ok {
		return
	}
	return
}

func (s *Server) onAccept() {
	conn, err := s.listener.Accept()
	if err != nil {
		s.handleError(err, nil)
		return
	}
	s.addSession(conn)
}

func (s *Server) Close() error {
	s.listener.Close()
	s.cancelCmd()
	close(s.cmdChan)
	return nil
}
func (s *Server) handleError(err error, session *Session) {
	s.options.Handler.OnError(err, session)
}
