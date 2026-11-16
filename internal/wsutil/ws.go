// Package wsutil is the small slice of WebSocket (RFC 6455) the live relay
// needs: an HTTP upgrade, masked client frames, and unmasked server frames.
// Text and binary frames are carried whole. The gateway does not look inside
// them, so a tool call is the same bytes on both ends.
package wsutil

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	OpCont  = 0x0
	OpText  = 0x1
	OpBin   = 0x2
	OpClose = 0x8
	OpPing  = 0x9
	OpPong  = 0xA

	maxFrame = 8 << 20
	wsGUID   = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

// Conn is one side of a websocket. mask is set when this side is the client
// and must mask every frame it writes.
type Conn struct {
	c      net.Conn
	r      *bufio.Reader
	mask   bool
	writeM sync.Mutex
}

// Accept hijacks an HTTP request that asked to upgrade and answers 101.
func Accept(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !headerHas(r.Header, "Upgrade", "websocket") {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return nil, fmt.Errorf("not a websocket upgrade")
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return nil, fmt.Errorf("missing websocket key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket not supported", http.StatusInternalServerError)
		return nil, fmt.Errorf("response cannot be hijacked")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, err = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err == nil {
		err = rw.Flush()
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Conn{c: conn, r: rw.Reader, mask: false}, nil
}

// Dial opens a client websocket. headers are added after the upgrade headers;
// a caller uses them for the vendor's own credential.
func Dial(ctx context.Context, rawURL string, headers http.Header) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "wss" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	var conn net.Conn
	plain := &net.Dialer{}
	switch u.Scheme {
	case "wss":
		d := tls.Dialer{NetDialer: plain, Config: &tls.Config{ServerName: u.Hostname()}}
		conn, err = d.DialContext(ctx, "tcp", host)
	case "ws":
		conn, err = plain.DialContext(ctx, "tcp", host)
	default:
		return nil, fmt.Errorf("websocket scheme %q", u.Scheme)
	}
	if err != nil {
		return nil, err
	}
	keyRaw := make([]byte, 16)
	if _, err := rand.Read(keyRaw); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyRaw)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n", path, u.Host, key)
	for k, vs := range headers {
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	b.WriteString("\r\n")
	if _, err := io.WriteString(conn, b.String()); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.Contains(status, " 101 ") {
		conn.Close()
		return nil, fmt.Errorf("websocket upgrade: %s", strings.TrimSpace(status))
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return &Conn{c: conn, r: br, mask: true}, nil
}

// Read returns the next data or close frame. Ping is answered with pong and
// not returned.
func (c *Conn) Read() (byte, []byte, error) {
	for {
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(c.r, hdr); err != nil {
			return 0, nil, err
		}
		op := hdr[0] & 0x0f
		masked := hdr[1]&0x80 != 0
		n := uint64(hdr[1] & 0x7f)
		switch n {
		case 126:
			ext := make([]byte, 2)
			if _, err := io.ReadFull(c.r, ext); err != nil {
				return 0, nil, err
			}
			n = uint64(binary.BigEndian.Uint16(ext))
		case 127:
			ext := make([]byte, 8)
			if _, err := io.ReadFull(c.r, ext); err != nil {
				return 0, nil, err
			}
			n = binary.BigEndian.Uint64(ext)
		}
		if n > maxFrame {
			return 0, nil, fmt.Errorf("websocket frame is %d bytes", n)
		}
		var mask []byte
		if masked {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(c.r, mask); err != nil {
				return 0, nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(c.r, payload); err != nil {
			return 0, nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch op {
		case OpPing:
			if err := c.Write(OpPong, payload); err != nil {
				return 0, nil, err
			}
		case OpPong:
		default:
			return op, payload, nil
		}
	}
}

// Write sends one frame. Client connections mask the payload.
func (c *Conn) Write(op byte, payload []byte) error {
	if len(payload) > maxFrame {
		return fmt.Errorf("websocket frame is %d bytes", len(payload))
	}
	c.writeM.Lock()
	defer c.writeM.Unlock()
	header := []byte{0x80 | op}
	var mask []byte
	if c.mask {
		mask = make([]byte, 4)
		if _, err := rand.Read(mask); err != nil {
			return err
		}
	}
	n := len(payload)
	switch {
	case n < 126:
		b := byte(n)
		if c.mask {
			b |= 0x80
		}
		header = append(header, b)
	case n < 65536:
		b := byte(126)
		if c.mask {
			b |= 0x80
		}
		header = append(header, b, byte(n>>8), byte(n))
	default:
		b := byte(127)
		if c.mask {
			b |= 0x80
		}
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		header = append(header, b)
		header = append(header, ext[:]...)
	}
	if c.mask {
		header = append(header, mask...)
		masked := make([]byte, n)
		for i := range payload {
			masked[i] = payload[i] ^ mask[i%4]
		}
		payload = masked
	}
	if _, err := c.c.Write(header); err != nil {
		return err
	}
	_, err := c.c.Write(payload)
	return err
}

// SetDeadline bounds reads and writes. Tests use it so a stuck relay fails
// the test instead of hanging it.
func (c *Conn) SetDeadline(t time.Time) error {
	return c.c.SetDeadline(t)
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	if c == nil || c.c == nil {
		return nil
	}
	return c.c.Close()
}

func headerHas(h http.Header, key, want string) bool {
	for _, v := range strings.Split(h.Get(key), ",") {
		if strings.EqualFold(strings.TrimSpace(v), want) {
			return true
		}
	}
	return false
}
