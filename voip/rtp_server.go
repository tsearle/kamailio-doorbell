package voip

import (
	"fmt"
	"net"
	"time"
)

type RtpServer struct {
	conn         *net.UDPConn
	dest         *net.UDPAddr
	shutdown     bool
	writeHandler func([]byte)
	name         string
}

func (r *RtpServer) Run() {
	for !r.shutdown {
		select {
		default:
			buf := make([]byte, 1500)
			err := r.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			if err != nil {
				fmt.Println("Failed to set read deadline:", err)
				continue
			}
			n, dest, err := r.conn.ReadFromUDP(buf)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if err != nil {
				fmt.Printf("Error reading from UDP: %v\n", err)
				continue
			}

			if r.dest == nil {
				r.dest = dest
			} else if r.dest.String() != dest.String() {
				fmt.Printf("Received packet from unexpected source %s\n", dest.String())
				continue
			}

			if r.writeHandler != nil {
				r.writeHandler(buf[:n])
			}
		}
	}
	fmt.Printf("Shutting down RtpServer %s\n", r.name)
}

func (r *RtpServer) Close() {
	r.shutdown = true
	_ = r.conn.Close()
}

func (r *RtpServer) GetPort() int {
	return r.conn.LocalAddr().(*net.UDPAddr).Port
}

func (r *RtpServer) SetWriteHandler(handler func([]byte)) {
	r.writeHandler = handler
}

func (r *RtpServer) Write(data []byte) (int, error) {
	if r.dest != nil {
		return r.conn.WriteToUDP(data, r.dest)
	}
	return 0, nil
}

func NewRtpServer(name string) (*RtpServer, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: 0,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		return nil, fmt.Errorf("NewRtpServer: Error listening: %v", err)
	}
	r := &RtpServer{
		conn:     conn,
		shutdown: false,
		name:     name,
	}
	go r.Run()
	return r, nil
}
