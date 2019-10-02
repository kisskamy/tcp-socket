package tcpSocket

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

// Conn wrap net.Conn
type Conn struct {
	sid        string
	rawConn    net.Conn
	sendCh     chan []byte
	done       chan error
	hbTimer    *time.Timer
	name       string
	messageCh  chan *Message
	hbInterval time.Duration
	hbTimeout  time.Duration
}

// GetName Get conn name
func (c *Conn) GetName() string {
	return c.name
}

// NewConn create new conn
func NewConn(c net.Conn, hbInterval time.Duration, hbTimeout time.Duration) *Conn {
	conn := &Conn{
		rawConn:    c,
		sendCh:     make(chan []byte, 100),
		done:       make(chan error),
		messageCh:  make(chan *Message, 100),
		hbInterval: hbInterval,
		hbTimeout:  hbTimeout,
	}

	conn.name = c.RemoteAddr().String()
	conn.hbTimer = time.NewTimer(conn.hbInterval)

	if conn.hbInterval == 0 {
		conn.hbTimer.Stop()
	}

	return conn
}

// Close close connection
func (c *Conn) Close() {
	c.hbTimer.Stop()
	_ = c.rawConn.Close()
}

// SendMessage send message
func (c *Conn) SendMessage(msg *Message) error {
	pkg, err := Encode(msg)
	if err != nil {
		log.Errorf("组包失败: " + err.Error())
		return err
	}

	c.sendCh <- pkg
	return nil
}

// writeCoroutine write coroutine
func (c *Conn) writeCoroutine(ctx context.Context) {
	hbData := make([]byte, 0)

	for {
		select {
		case <-ctx.Done():
			return

		case pkt := <-c.sendCh:

			if pkt == nil {
				continue
			}

			if _, err := c.rawConn.Write(pkt); err != nil {
				c.done <- err
				log.Errorf("推送消息失败: " + err.Error())
			}

		case <-c.hbTimer.C:
			hbMessage := NewMessage(MsgHeartbeat, hbData)
			_ = c.SendMessage(hbMessage)
		}
	}
}

// readCoroutine read coroutine
func (c *Conn) readCoroutine(ctx context.Context) {

	for {
		select {
		case <-ctx.Done():
			return

		default:
			// 设置超时
			if c.hbInterval > 0 {
				err := c.rawConn.SetReadDeadline(time.Now().Add(c.hbTimeout))
				if err != nil {
					c.done <- err
					log.Errorf("设置超时失败: " + err.Error())
					continue
				}
			}
			// 读取长度
			buf := make([]byte, 4)
			_, err := io.ReadFull(c.rawConn, buf)
			if err != nil {
				c.done <- err
				log.Errorf("读取失败: " + err.Error())
				continue
			}
			size, err := strconv.Atoi(string(buf))
			if size == 0 {
				c.done <- errors.New("data header error")
				log.Errorf("数据大小不正确")
				continue
			}

			bufReader := bytes.NewReader(buf)
			var dataSize int32
			err = binary.Read(bufReader, binary.LittleEndian, &dataSize)
			if err != nil {
				c.done <- err
				log.Errorf("读取失败: " + err.Error())
				continue
			}

			// 读取数据
			if dataSize > 1000000 {
				c.done <- errors.New("out of memory")
				log.Errorf("dataSize变量内存溢出")
				continue
			}
			dataBuf := make([]byte, dataSize)
			_, err = io.ReadFull(c.rawConn, dataBuf)
			if err != nil {
				c.done <- err
				log.Errorf("读取失败: " + err.Error())
				continue
			}

			// 解码
			msg, err := Decode(dataBuf)
			if err != nil {
				c.done <- err
				log.Errorf("解包失败: " + err.Error())
				continue
			}

			// 设置心跳timer
			if c.hbInterval > 0 {
				c.hbTimer.Reset(c.hbInterval)
			}

			if msg.GetID() == MsgHeartbeat {
				continue
			}

			c.messageCh <- msg
		}
	}
}
