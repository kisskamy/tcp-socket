package tcpsocket

import (
	"context"
	"errors"
	"github.com/kisskamy/zxylog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SocketService struct
type SocketService struct {
	onMessage    func(*Session, *Message)
	onConnect    func(*Session)
	onDisconnect func(*Session, error)
	sessions     *sync.Map
	hbInterval   time.Duration
	hbTimeout    time.Duration
	laddr        string
	status       int
	listener     net.Listener
	stopCh       chan error
}

var log *zxylog.ZxyLog

//获取当前执行文件路径
func GetBinDir() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err.Error())
	}
	return strings.Replace(dir, "\\", "/", -1)
}

// NewSocketService create a new socket service
func NewSocketService(laddr string) (*SocketService, error) {

	log = zxylog.NewZxyLog(GetBinDir()+"/../logs", "TcpServer")
	log.SetLevel(zxylog.INFO)

	l, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Errorf("监听失败: " + err.Error())
		return nil, err
	}

	s := &SocketService{
		sessions:   &sync.Map{},
		stopCh:     make(chan error),
		hbInterval: 0 * time.Second,
		hbTimeout:  0 * time.Second,
		laddr:      laddr,
		status:     STInited,
		listener:   l,
	}

	return s, nil
}

// RegMessageHandler register message handler
func (s *SocketService) RegMessageHandler(handler func(*Session, *Message)) {
	s.onMessage = handler
}

// RegConnectHandler register connect handler
func (s *SocketService) RegConnectHandler(handler func(*Session)) {
	s.onConnect = handler
}

// RegDisconnectHandler register disconnect handler
func (s *SocketService) RegDisconnectHandler(handler func(*Session, error)) {
	s.onDisconnect = handler
}

// Serv Start socket service
func (s *SocketService) Serv() {

	s.status = STRunning
	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		s.status = STStop
		cancel()
		_ = s.listener.Close()
	}()

	go s.acceptHandler(ctx)

	for {
		select {

		case <-s.stopCh:
			log.Infof("服务停止...")
			return
		}
	}
}

func (s *SocketService) acceptHandler(ctx context.Context) {
	for {
		c, err := s.listener.Accept()
		if err != nil {
			s.stopCh <- err
			log.Errorf("Accept错误: " + err.Error())
			return
		}

		go s.connectHandler(ctx, c)
	}
}

func (s *SocketService) connectHandler(ctx context.Context, c net.Conn) {
	conn := NewConn(c, s.hbInterval, s.hbTimeout)
	session := NewSession(conn)
	s.sessions.Store(session.GetSessionID(), session)

	connctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
		conn.Close()
		s.sessions.Delete(session.GetSessionID())
	}()

	go conn.readCoroutine(connctx)
	go conn.writeCoroutine(connctx)

	if s.onConnect != nil {
		s.onConnect(session)
	}

	for {
		select {
		case err := <-conn.done:

			if s.onDisconnect != nil {
				s.onDisconnect(session, err)
			}
			return

		case msg := <-conn.messageCh:
			if s.onMessage != nil {
				s.onMessage(session, msg)
			}
		}
	}
}

// GetStatus get socket service status
func (s *SocketService) GetStatus() int {
	return s.status
}

// Stop stop socket service with reason
func (s *SocketService) Stop(reason string) {
	s.stopCh <- errors.New(reason)
}

// SetHeartBeat set heart beat
func (s *SocketService) SetHeartBeat(hbInterval time.Duration, hbTimeout time.Duration) error {
	if s.status == STRunning {
		return errors.New("Can't set heart beat on service running")
	}

	s.hbInterval = hbInterval
	s.hbTimeout = hbTimeout

	return nil
}

// GetConnsCount get connect count
func (s *SocketService) GetConnsCount() int {
	var count int
	s.sessions.Range(func(k, v interface{}) bool {
		count++
		return true
	})
	return count
}

// Unicast Unicast with session ID
func (s *SocketService) Unicast(sid string, msg *Message) {
	v, ok := s.sessions.Load(sid)
	if ok {
		session := v.(*Session)
		err := session.GetConn().SendMessage(msg)
		if err != nil {
			log.Errorf("推送失败: " + err.Error())
			return
		}
	}
}

// Broadcast Broadcast to all connections
func (s *SocketService) Broadcast(msg *Message) {
	s.sessions.Range(func(k, v interface{}) bool {
		s := v.(*Session)
		if err := s.GetConn().SendMessage(msg); err != nil {
			log.Errorf(s.GetUserID() + "推送失败: " + err.Error())
		}
		return true
	})
}
