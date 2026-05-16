package mqttdynsec

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	controlRequestTopic  = "$CONTROL/dynamic-security/v1"
	controlResponseTopic = "$CONTROL/dynamic-security/v1/response"
	mqttProtocolLevel    = 4
)

type Provisioner interface {
	EnsureServerPublisher(ctx context.Context, username, password string) error
	UpsertDevice(ctx context.Context, deviceID, deviceSecret string) error
	DisableDevice(ctx context.Context, deviceID string) error
}

type Config struct {
	Address     string
	ClientID    string
	Username    string
	Password    string
	KeepAlive   time.Duration
	DialTimeout time.Duration
}

type Client struct {
	cfg Config
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, fmt.Errorf("missing mqtt address")
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		cfg.ClientID = "xmdm-dynsec"
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.KeepAlive <= 0 {
		cfg.KeepAlive = 30 * time.Second
	}
	return &Client{cfg: cfg}, nil
}

func (c *Client) UpsertDevice(ctx context.Context, deviceID, deviceSecret string) error {
	if strings.TrimSpace(deviceID) == "" || strings.TrimSpace(deviceSecret) == "" {
		return fmt.Errorf("missing device credentials")
	}
	commandTopic := fmt.Sprintf("devices/%s/commands", deviceID)
	deviceRole := "xmdm-device-command"
	if err := c.ensureRole(ctx, deviceRole,
		map[string]any{
			"acltype":  "subscribeLiteral",
			"topic":    commandTopic,
			"allow":    true,
			"priority": 100,
		},
		map[string]any{
			"acltype":  "publishClientReceive",
			"topic":    commandTopic,
			"allow":    true,
			"priority": 100,
		},
	); err != nil {
		return err
	}

	if err := c.runCommand(ctx, map[string]any{
		"command":  "createClient",
		"username": deviceID,
		"password": deviceSecret,
		"clientid": deviceID,
		"roles":    []map[string]any{{"rolename": deviceRole, "priority": 100}},
	}, "already exists"); err != nil {
		return err
	}

	if err := c.runCommand(ctx, map[string]any{
		"command":  "modifyClient",
		"username": deviceID,
		"password": deviceSecret,
		"clientid": deviceID,
		"roles":    []map[string]any{{"rolename": deviceRole, "priority": 100}},
	}); err != nil {
		return err
	}

	return nil
}

func (c *Client) EnsureServerPublisher(ctx context.Context, username, password string) error {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return fmt.Errorf("missing server publisher credentials")
	}

	serverRole := "xmdm-server-publisher"
	if err := c.ensureRole(ctx, serverRole,
		map[string]any{
			"acltype":  "publishClientSend",
			"topic":    "devices/+/commands",
			"allow":    true,
			"priority": 100,
		},
	); err != nil {
		return err
	}

	if err := c.runCommand(ctx, map[string]any{
		"command":  "createClient",
		"username": username,
		"clientid": username,
		"password": password,
		"roles":    []map[string]any{{"rolename": serverRole, "priority": 100}},
	}, "already exists"); err != nil {
		return err
	}

	if err := c.runCommand(ctx, map[string]any{
		"command":  "modifyClient",
		"username": username,
		"clientid": username,
		"password": password,
		"roles":    []map[string]any{{"rolename": serverRole, "priority": 100}},
	}); err != nil {
		return err
	}

	return nil
}

func (c *Client) DisableDevice(ctx context.Context, deviceID string) error {
	if strings.TrimSpace(deviceID) == "" {
		return fmt.Errorf("missing device id")
	}
	var errs []error
	if err := c.runCommand(ctx, map[string]any{
		"command":  "disableClient",
		"username": deviceID,
	}, "not found"); err != nil {
		errs = append(errs, err)
	}
	if err := c.runCommand(ctx, map[string]any{
		"command":  "deleteClient",
		"username": deviceID,
	}, "not found"); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (c *Client) ensureRole(ctx context.Context, roleName string, acls ...map[string]any) error {
	if err := c.runCommand(ctx, map[string]any{
		"command":  "createRole",
		"rolename": roleName,
	}, "already exists"); err != nil {
		return err
	}
	for _, acl := range acls {
		if err := c.runCommand(ctx, map[string]any{
			"command":  "addRoleACL",
			"rolename": roleName,
			"acltype":  acl["acltype"],
			"topic":    acl["topic"],
			"allow":    acl["allow"],
			"priority": acl["priority"],
		}, "already exists"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) runCommand(ctx context.Context, command map[string]any, toleratedErrors ...string) error {
	conn, err := dialMQTT(ctx, "tcp", c.cfg.Address, c.cfg.DialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.cfg.DialTimeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	if err := writePacket(conn, connectPacket(c.cfg)); err != nil {
		return err
	}
	connack, err := readPacket(conn)
	if err != nil {
		return err
	}
	if len(connack) < 4 || connack[0] != 0x20 || connack[3] != 0x00 {
		return fmt.Errorf("mqtt connection rejected")
	}

	if err := writePacket(conn, subscribePacket(controlResponseTopic, 1)); err != nil {
		return err
	}
	if _, err := readPacket(conn); err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"commands": []map[string]any{command},
	})
	if err != nil {
		return err
	}
	if err := writePacket(conn, publishPacket(controlRequestTopic, payload)); err != nil {
		return err
	}

	for {
		packet, err := readPacket(conn)
		if err != nil {
			return err
		}
		if len(packet) == 0 {
			continue
		}
		if packet[0]&0xF0 != 0x30 {
			continue
		}
		topic, next, err := readMQTTString(packet, 1+remainingLengthBytes(packet[1:]))
		if err != nil {
			return err
		}
		if topic != controlResponseTopic {
			continue
		}
		body := packet[next:]
		var resp dynsecResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return err
		}
		if len(resp.Responses) == 0 {
			return fmt.Errorf("missing dynsec response")
		}
		if respErr := resp.Responses[0].Error; respErr != "" {
			if containsAny(respErr, toleratedErrors...) {
				return nil
			}
			return fmt.Errorf("dynsec command %s failed: %s", resp.Responses[0].Command, respErr)
		}
		return nil
	}
}

type dynsecResponse struct {
	Responses []struct {
		Command string `json:"command"`
		Error   string `json:"error,omitempty"`
	} `json:"responses"`
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(strings.ToLower(value), strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

var dialMQTT = func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	return dialer.DialContext(ctx, network, address)
}

func connectPacket(cfg Config) []byte {
	var body []byte
	body = append(body, encodeString("MQTT")...)
	body = append(body, mqttProtocolLevel)

	flags := byte(0x02)
	if cfg.Username != "" {
		flags |= 0x80
	}
	if cfg.Password != "" {
		flags |= 0x40
	}
	body = append(body, flags)
	keepAlive := uint16(cfg.KeepAlive / time.Second)
	body = append(body, byte(keepAlive>>8), byte(keepAlive))

	var payload []byte
	payload = append(payload, encodeString(cfg.ClientID)...)
	if cfg.Username != "" {
		payload = append(payload, encodeString(cfg.Username)...)
	}
	if cfg.Password != "" {
		payload = append(payload, encodeString(cfg.Password)...)
	}

	return append(append([]byte{0x10}, encodeRemainingLength(len(body)+len(payload))...), append(body, payload...)...)
}

func subscribePacket(topic string, packetID uint16) []byte {
	var body []byte
	body = append(body, byte(packetID>>8), byte(packetID))
	body = append(body, encodeString(topic)...)
	body = append(body, 0x00)
	return append(append([]byte{0x82}, encodeRemainingLength(len(body))...), body...)
}

func publishPacket(topic string, payload []byte) []byte {
	var body []byte
	body = append(body, encodeString(topic)...)
	body = append(body, payload...)
	return append(append([]byte{0x30}, encodeRemainingLength(len(body))...), body...)
}

func encodeString(value string) []byte {
	data := []byte(value)
	out := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(out[:2], uint16(len(data)))
	copy(out[2:], data)
	return out
}

func encodeRemainingLength(n int) []byte {
	if n < 0 {
		return []byte{0}
	}
	var encoded []byte
	for {
		digit := byte(n % 128)
		n /= 128
		if n > 0 {
			digit |= 0x80
		}
		encoded = append(encoded, digit)
		if n == 0 {
			break
		}
	}
	return encoded
}

func writePacket(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

func readPacket(r io.Reader) ([]byte, error) {
	header := make([]byte, 1)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	remaining, err := readRemainingLength(r)
	if err != nil {
		return nil, err
	}
	packet := make([]byte, 1+len(remaining))
	packet[0] = header[0]
	copy(packet[1:], remaining)
	body := make([]byte, decodeRemainingLength(remaining))
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return append(packet, body...), nil
}

func readRemainingLength(r io.Reader) ([]byte, error) {
	var encoded []byte
	for i := 0; i < 4; i++ {
		b := make([]byte, 1)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, err
		}
		encoded = append(encoded, b[0])
		if b[0]&0x80 == 0 {
			return encoded, nil
		}
	}
	return nil, fmt.Errorf("mqtt remaining length too large")
}

func decodeRemainingLength(encoded []byte) int {
	multiplier := 1
	value := 0
	for _, b := range encoded {
		value += int(b&127) * multiplier
		multiplier *= 128
	}
	return value
}

func remainingLengthBytes(encoded []byte) int {
	for i, b := range encoded {
		if b&0x80 == 0 {
			return i + 1
		}
	}
	return len(encoded)
}

func readMQTTString(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", offset, fmt.Errorf("mqtt string length truncated")
	}
	length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+length > len(data) {
		return "", offset, fmt.Errorf("mqtt string truncated")
	}
	return string(data[offset : offset+length]), offset + length, nil
}
