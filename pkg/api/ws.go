package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"
)

const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type WSMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

type WSClient struct {
	conn *net.TCPConn
	rw   *bufio.ReadWriter
	send chan []byte
	hub  *WSHub
}

type WSHub struct {
	clients map[*WSClient]bool
	broadcast chan []byte
	register chan *WSClient
	unregister chan *WSClient
	mu sync.RWMutex
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *WSHub) Broadcast(event string, data interface{}) {
	msg := WSMessage{
		Event: event,
		Data:  data,
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.broadcast <- bytes
}

func (h *WSHub) HandleWebSocket(w http.ResponseWriter, r *http.Request, onMessage func(client *WSClient, msg []byte)) {
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Not a websocket handshake", http.StatusBadRequest)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	hash := sha1.New()
	hash.Write([]byte(key + magicGUID))
	acceptKey := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		conn.Close()
		return
	}

	// Write handshake response
	res := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

	rw.WriteString(res)
	rw.Flush()

	client := &WSClient{
		conn: tcpConn,
		rw:   rw,
		send: make(chan []byte, 256),
		hub:  h,
	}

	h.register <- client

	go client.writePump()
	go client.readPump(onMessage)
}

func (c *WSClient) readPump(onMessage func(client *WSClient, msg []byte)) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(c.rw, header); err != nil {
			break
		}

		fin := (header[0] & 0x80) != 0
		opcode := header[0] & 0x0F
		masked := (header[1] & 0x80) != 0
		payloadLen := uint64(header[1] & 0x7F)

		if opcode == 0x08 { // Connection Close
			break
		}

		if payloadLen == 126 {
			extended := make([]byte, 2)
			if _, err := io.ReadFull(c.rw, extended); err != nil {
				break
			}
			payloadLen = uint64(binary.BigEndian.Uint16(extended))
		} else if payloadLen == 127 {
			extended := make([]byte, 8)
			if _, err := io.ReadFull(c.rw, extended); err != nil {
				break
			}
			payloadLen = binary.BigEndian.Uint64(extended)
		}

		var maskKey []byte
		if masked {
			maskKey = make([]byte, 4)
			if _, err := io.ReadFull(c.rw, maskKey); err != nil {
				break
			}
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(c.rw, payload); err != nil {
			break
		}

		if masked {
			for i := uint64(0); i < payloadLen; i++ {
				payload[i] ^= maskKey[i%4]
			}
		}

		if fin && (opcode == 0x01 || opcode == 0x02) { // Text or Binary frame
			if onMessage != nil {
				onMessage(c, payload)
			}
		}
	}
}

func (c *WSClient) writePump() {
	defer c.conn.Close()

	for msg := range c.send {
		frame := makeWSFrame(msg)
		if _, err := c.rw.Write(frame); err != nil {
			break
		}
		if err := c.rw.Flush(); err != nil {
			break
		}
	}
}

func makeWSFrame(payload []byte) []byte {
	length := len(payload)
	var header []byte

	if length <= 125 {
		header = make([]byte, 2)
		header[0] = 0x81 // Text frame, FIN
		header[1] = byte(length)
	} else if length <= 65535 {
		header = make([]byte, 4)
		header[0] = 0x81
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(length))
	} else {
		header = make([]byte, 10)
		header[0] = 0x81
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(length))
	}

	return append(header, payload...)
}
