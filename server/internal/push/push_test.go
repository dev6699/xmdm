package push

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestTopicForDevice(t *testing.T) {
	if got := TopicForDevice("device-123"); got != "devices/device-123/commands" {
		t.Fatalf("unexpected topic: %s", got)
	}
}

func TestPublishCommand(t *testing.T) {
	ctx := context.Background()

	cfg := MQTTConfig{
		Address:     "ignored",
		ClientID:    "xmdm-test",
		KeepAlive:   30 * time.Second,
		DialTimeout: 5 * time.Second,
	}
	pub := &MQTTPublisher{cfg: cfg}

	conn := &recordingConn{readBuf: *bytes.NewBuffer([]byte{0x20, 0x02, 0x00, 0x00, 0x40, 0x02, 0x00, 0x01})}
	oldDial := dialMQTT
	dialMQTT = func(context.Context, string, string, time.Duration) (net.Conn, error) {
		return conn, nil
	}
	defer func() { dialMQTT = oldDial }()

	if err := pub.PublishCommand(ctx, "device-123", CommandMessage{
		Type:      "reboot",
		CommandID: "cmd-1",
		DeviceID:  "device-123",
		Payload:   map[string]any{"force": true},
		CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	packet := conn.writes.Bytes()
	if len(packet) == 0 || packet[0] != 0x10 {
		t.Fatalf("expected connect packet, got %x", packet)
	}
	firstRemaining, firstConsumed := decodePacketRemainingLength(packet[1:])
	offset := 1 + firstConsumed + firstRemaining
	if offset >= len(packet) {
		t.Fatalf("missing publish packet: %x", packet)
	}
	publish := extractPublishedPacket(t, packet[offset:])
	if publish.qos != 1 {
		t.Fatalf("unexpected qos: %d", publish.qos)
	}
	if publish.packetID != 1 {
		t.Fatalf("unexpected packet id: %d", publish.packetID)
	}
	if publish.topic != "devices/device-123/commands" {
		t.Fatalf("unexpected topic: %s", publish.topic)
	}
	var decoded CommandMessage
	if err := json.Unmarshal(publish.payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.Type != "reboot" || decoded.CommandID != "cmd-1" || decoded.DeviceID != "device-123" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}

func TestPublishCommandRejectsEmptyDeviceID(t *testing.T) {
	pub := &MQTTPublisher{cfg: MQTTConfig{Address: "127.0.0.1:1883", ClientID: "xmdm"}}
	if err := pub.PublishCommand(context.Background(), "", CommandMessage{Type: "reboot"}); err == nil {
		t.Fatalf("expected error for empty device id")
	}
}

type publishedPacket struct {
	topic    string
	packetID uint16
	qos      byte
	payload  []byte
}

type recordingConn struct {
	readBuf bytes.Buffer
	writes  bytes.Buffer
	closed  bool
}

func (c *recordingConn) Read(p []byte) (int, error) {
	return c.readBuf.Read(p)
}

func (c *recordingConn) Write(p []byte) (int, error) {
	return c.writes.Write(p)
}

func (c *recordingConn) Close() error {
	c.closed = true
	return nil
}

func (c *recordingConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (c *recordingConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (c *recordingConn) SetDeadline(time.Time) error      { return nil }
func (c *recordingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordingConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return string(a) }
func (a dummyAddr) String() string  { return string(a) }

func extractPublishedPacket(t *testing.T, packet []byte) publishedPacket {
	t.Helper()
	if len(packet) < 2 {
		t.Fatalf("packet too short")
	}
	qos := byte(packet[0]>>1) & 0x03
	remaining, consumed := decodePacketRemainingLength(packet[1:])
	offset := 1 + consumed
	if len(packet) < offset+remaining {
		t.Fatalf("packet truncated")
	}
	topic, next, err := readMQTTString(packet, offset)
	if err != nil {
		t.Fatalf("read topic: %v", err)
	}
	packetID := uint16(0)
	if qos > 0 {
		if len(packet) < next+2 {
			t.Fatalf("missing packet id")
		}
		packetID = uint16(packet[next])<<8 | uint16(packet[next+1])
		next += 2
	}
	return publishedPacket{
		topic:    topic,
		packetID: packetID,
		qos:      qos,
		payload:  packet[next : offset+remaining],
	}
}

func decodePacketRemainingLength(data []byte) (int, int) {
	multiplier := 1
	value := 0
	consumed := 0
	for {
		b := data[consumed]
		consumed++
		value += int(b&127) * multiplier
		if b&0x80 == 0 {
			break
		}
		multiplier *= 128
	}
	return value, consumed
}
