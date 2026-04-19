package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/aykutsp/ntp-server/internal/ntp"
)

type result struct {
	Server      string    `json:"server"`
	Stratum     uint8     `json:"stratum"`
	ReferenceID string    `json:"referenceId"`
	ReceiveTime time.Time `json:"receiveTime"`
	Transmit    time.Time `json:"transmitTime"`
	OffsetMs    float64   `json:"offsetMs"`
	RTTMs       float64   `json:"rttMs"`
}

func main() {
	server := flag.String("server", "127.0.0.1:12300", "NTP server address")
	timeout := flag.Duration("timeout", 2*time.Second, "request timeout")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", *server)
	if err != nil {
		fatalf("resolve failed: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(*timeout))

	var req [48]byte
	req[0] = (0 << 6) | (4 << 3) | 3
	t1 := time.Now().UTC()
	copy(req[40:48], toBytes(ntp.ToTimestamp(t1)))

	if _, err := conn.Write(req[:]); err != nil {
		fatalf("write failed: %v", err)
	}

	var resp [128]byte
	n, err := conn.Read(resp[:])
	if err != nil {
		fatalf("read failed: %v", err)
	}
	if n < 48 {
		fatalf("short response: %d bytes", n)
	}

	t4 := time.Now().UTC()
	t2 := ntp.ReadTimestamp(resp[:], 32)
	t3 := ntp.ReadTimestamp(resp[:], 40)

	offset := ((t2.Sub(t1)) + (t3.Sub(t4))) / 2
	rtt := (t4.Sub(t1)) - (t3.Sub(t2))
	if rtt < 0 {
		rtt = 0
	}

	res := result{
		Server:      *server,
		Stratum:     resp[1],
		ReferenceID: sanitizeRefID(resp[12:16]),
		ReceiveTime: t2,
		Transmit:    t3,
		OffsetMs:    float64(offset) / float64(time.Millisecond),
		RTTMs:       float64(rtt) / float64(time.Millisecond),
	}

	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
}

func sanitizeRefID(b []byte) string {
	// If all bytes are printable ASCII, return as string (e.g. "LOCL", "GOOG")
	allPrint := true
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			allPrint = false
			break
		}
	}
	if allPrint {
		return strings.TrimRight(string(b), "\x00")
	}
	// Otherwise it's an IP address (stratum 2+) — format as dotted decimal
	if len(b) == 4 {
		return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
	}
	return fmt.Sprintf("%x", b)
}

func toBytes(v uint64) []byte {
	return []byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
