package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	feastconn "github.com/user/feastgo/pkg/conn"
	"github.com/user/feastgo/pkg/protocol"
)

type handshakePacket struct {
	ProtocolVersion int32
	ServerAddress   string
	ServerPort      uint16
	NextState       int32
}

func (p *handshakePacket) PacketID() int32 { return 0x00 }
func (p *handshakePacket) Marshal(w *protocol.Writer) error {
	if err := w.WriteVarInt(p.ProtocolVersion); err != nil {
		return err
	}
	if err := w.WriteString(p.ServerAddress); err != nil {
		return err
	}
	if err := w.WriteShort(int16(p.ServerPort)); err != nil {
		return err
	}
	return w.WriteVarInt(p.NextState)
}
func (p *handshakePacket) Unmarshal(_ *protocol.Reader) error { return nil }

type statusRequestPacket struct{}

func (p *statusRequestPacket) PacketID() int32                    { return 0x00 }
func (p *statusRequestPacket) Marshal(_ *protocol.Writer) error   { return nil }
func (p *statusRequestPacket) Unmarshal(_ *protocol.Reader) error { return nil }

type pingRequestPacket struct{ Payload int64 }

func (p *pingRequestPacket) PacketID() int32                    { return 0x01 }
func (p *pingRequestPacket) Marshal(w *protocol.Writer) error   { return w.WriteLong(p.Payload) }
func (p *pingRequestPacket) Unmarshal(_ *protocol.Reader) error { return nil }

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <host> [port]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	host := args[0]
	port := "25565"
	if len(args) > 1 {
		port = args[1]
	}
	portNum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		log.Fatalf("invalid port: %v", err)
	}

	nc, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer nc.Close()

	c := feastconn.New(nc)
	if err := c.WritePacket(&handshakePacket{ProtocolVersion: 765, ServerAddress: host, ServerPort: uint16(portNum), NextState: 1}); err != nil {
		log.Fatalf("handshake failed: %v", err)
	}
	if err := c.WritePacket(&statusRequestPacket{}); err != nil {
		log.Fatalf("status request failed: %v", err)
	}

	status, err := c.ReadPacket()
	if err != nil {
		log.Fatalf("status read failed: %v", err)
	}
	if status.ID != 0x00 {
		log.Fatalf("unexpected status packet id: 0x%02x", status.ID)
	}
	r := protocol.NewReader(bytes.NewReader(status.Data))
	jsonStatus, err := r.ReadString()
	if err != nil {
		log.Fatalf("status decode failed: %v", err)
	}
	fmt.Printf("Status: %s\n", jsonStatus)

	now := time.Now().UnixMilli()
	if err := c.WritePacket(&pingRequestPacket{Payload: now}); err != nil {
		log.Fatalf("ping request failed: %v", err)
	}
	pong, err := c.ReadPacket()
	if err != nil {
		log.Fatalf("pong read failed: %v", err)
	}
	if pong.ID != 0x01 {
		log.Fatalf("unexpected pong packet id: 0x%02x", pong.ID)
	}
	pr := protocol.NewReader(bytes.NewReader(pong.Data))
	payload, err := pr.ReadLong()
	if err != nil {
		log.Fatalf("pong decode failed: %v", err)
	}
	fmt.Printf("Latency: %dms\n", time.Now().UnixMilli()-payload)
}
