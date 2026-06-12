package push

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

const mqttProtocolLevel = 4

func publishQoS1(ctx context.Context, cfg MQTTConfig, topic string, payload []byte) error {
	conn, err := dialMQTT(ctx, "tcp", cfg.Address, cfg.DialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(cfg.DialTimeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	if err := writePacket(conn, connectPacket(cfg)); err != nil {
		return err
	}
	connack, err := readPacket(conn)
	if err != nil {
		return err
	}
	if len(connack) < 4 || connack[0] != 0x20 || connack[3] != 0x00 {
		return fmt.Errorf("mqtt connection rejected")
	}
	if err := conn.SetDeadline(phaseDeadline(ctx, cfg.DialTimeout)); err != nil {
		return err
	}

	const packetID uint16 = 1
	if err := writePacket(conn, publishPacket(topic, payload, 1, packetID)); err != nil {
		return err
	}
	puback, err := readPacket(conn)
	if err != nil {
		return err
	}
	if len(puback) < 4 || puback[0] != 0x40 || puback[1] != 0x02 || readPacketID(puback[2:]) != packetID {
		return fmt.Errorf("mqtt publish not acknowledged")
	}
	if err := writePacket(conn, []byte{0xE0, 0x00}); err != nil {
		return err
	}
	return nil
}

func phaseDeadline(ctx context.Context, timeout time.Duration) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Now().Add(timeout)
}

func connectPacket(cfg MQTTConfig) []byte {
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

func publishPacket(topic string, payload []byte, qos byte, packetID uint16) []byte {
	var body []byte
	body = append(body, encodeString(topic)...)
	if qos > 0 {
		body = append(body, byte(packetID>>8), byte(packetID))
	}
	body = append(body, payload...)
	return append(append([]byte{0x30 | ((qos & 0x03) << 1)}, encodeRemainingLength(len(body))...), body...)
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

func readPacketID(data []byte) uint16 {
	if len(data) < 2 {
		return 0
	}
	return uint16(data[0])<<8 | uint16(data[1])
}
