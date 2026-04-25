package mqttdynsec

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

func TestUpsertDevicePublishesDynsecCommand(t *testing.T) {
	conns := []*recordingConn{
		newRecordingConn(`{"responses":[{"command":"createRole"}]}`),
		newRecordingConn(`{"responses":[{"command":"addRoleACL"}]}`),
		newRecordingConn(`{"responses":[{"command":"addRoleACL"}]}`),
		newRecordingConn(`{"responses":[{"command":"createClient"}]}`),
	}
	oldDial := dialMQTT
	var dialCount int
	dialMQTT = func(context.Context, string, string, time.Duration) (net.Conn, error) {
		if dialCount >= len(conns) {
			t.Fatalf("unexpected dial count: %d", dialCount)
		}
		conn := conns[dialCount]
		dialCount++
		return conn, nil
	}
	defer func() { dialMQTT = oldDial }()

	client, err := New(Config{
		Address:     "broker:1883",
		ClientID:    "admin-client",
		Username:    "admin",
		Password:    "secret",
		KeepAlive:   30 * time.Second,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.UpsertDevice(context.Background(), "device-123", "device-secret"); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	if !bytes.Contains(conns[len(conns)-1].writes.Bytes(), []byte(controlRequestTopic)) {
		t.Fatalf("missing control request topic: %x", conns[len(conns)-1].writes.Bytes())
	}
	if !bytes.Contains(conns[len(conns)-1].writes.Bytes(), []byte(`"device-123"`)) {
		t.Fatalf("missing device id in payload: %s", conns[len(conns)-1].writes.Bytes())
	}
}

func TestDisableDeviceSendsDeleteCommands(t *testing.T) {
	conns := []*recordingConn{
		newRecordingConn(`{"responses":[{"command":"disableClient"}]}`),
		newRecordingConn(`{"responses":[{"command":"deleteClient"}]}`),
	}
	oldDial := dialMQTT
	var dialCount int
	dialMQTT = func(context.Context, string, string, time.Duration) (net.Conn, error) {
		if dialCount >= len(conns) {
			t.Fatalf("unexpected dial count: %d", dialCount)
		}
		conn := conns[dialCount]
		dialCount++
		return conn, nil
	}
	defer func() { dialMQTT = oldDial }()

	client, err := New(Config{Address: "broker:1883"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.DisableDevice(context.Background(), "device-123"); err != nil {
		t.Fatalf("disable device: %v", err)
	}
}

func newRecordingConn(response string) *recordingConn {
	packets := append([]byte{0x20, 0x02, 0x00, 0x00, 0x90, 0x03, 0x00, 0x01, 0x00}, publishPacket(controlResponseTopic, []byte(response))...)
	return &recordingConn{readBuf: *bytes.NewBuffer(packets)}
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
