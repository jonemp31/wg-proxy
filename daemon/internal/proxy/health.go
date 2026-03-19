package proxy

import (
	"fmt"
	"io"
	"net"
	"time"
)

func CheckHealth(port int, username, password string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// SOCKS5 greeting: VER=5, NMETHODS=1, METHOD=0x02 (username/password)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return fmt.Errorf("send greeting: %w", err)
	}

	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read greeting response: %w", err)
	}
	if buf[0] != 0x05 || buf[1] != 0x02 {
		return fmt.Errorf("unexpected method: %x", buf)
	}

	// Username/password auth (RFC 1929)
	auth := []byte{0x01, byte(len(username))}
	auth = append(auth, []byte(username)...)
	auth = append(auth, byte(len(password)))
	auth = append(auth, []byte(password)...)
	if _, err := conn.Write(auth); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read auth response: %w", err)
	}
	if buf[1] != 0x00 {
		return fmt.Errorf("auth failed: status %d", buf[1])
	}

	return nil
}
