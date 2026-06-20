package push

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

const deviceCommandTopicPrefix = "devices"

type CommandMessage struct {
	Type      string         `json:"type"`
	CommandID string         `json:"commandId,omitempty"`
	TenantID  string         `json:"tenantId,omitempty"`
	DeviceID  string         `json:"deviceId,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"createdAt,omitempty"`
}

type Publisher interface {
	PublishCommand(ctx context.Context, deviceID string, message CommandMessage) error
}

type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

type MQTTConfig struct {
	Address     string
	ClientID    string
	Username    string
	Password    string
	KeepAlive   time.Duration
	DialTimeout time.Duration
}

var dialMQTT = func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	return dialer.DialContext(ctx, network, address)
}

type MQTTPublisher struct {
	cfg MQTTConfig
}

func NewMQTTPublisher(cfg MQTTConfig) (*MQTTPublisher, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, fmt.Errorf("missing mqtt address")
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		cfg.ClientID = "xmdm-server"
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.KeepAlive <= 0 {
		cfg.KeepAlive = 30 * time.Second
	}
	return &MQTTPublisher{cfg: cfg}, nil
}

func TopicForDevice(deviceID string) string {
	return deviceCommandTopic(deviceID)
}

func (p *MQTTPublisher) PublishCommand(ctx context.Context, deviceID string, message CommandMessage) error {
	if p == nil {
		return fmt.Errorf("missing mqtt publisher")
	}
	if strings.TrimSpace(deviceID) == "" {
		return fmt.Errorf("missing device id")
	}
	if strings.TrimSpace(message.Type) == "" {
		return fmt.Errorf("missing command type")
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return publishQoS1(ctx, p.cfg, deviceCommandTopic(deviceID), payload)
}

func (p *MQTTPublisher) HealthCheck(ctx context.Context) error {
	if p == nil {
		return fmt.Errorf("missing mqtt publisher")
	}
	return publishQoS1(ctx, p.cfg, deviceCommandTopic("__health__"), []byte(`{"type":"health-check"}`))
}

func deviceCommandTopic(deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	return deviceCommandTopicPrefix + "/" + deviceID + "/commands"
}
