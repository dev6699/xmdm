package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the configuration structure
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Postgres   PostgresConfig   `yaml:"postgres"`
	MQTT       MQTTConfig        `yaml:"mqtt"`
	ObjectStore ObjectStoreConfig `yaml:"objectStorage"`
	Admin      AdminConfig      `yaml:"admin"`
}

type ServerConfig struct {
	Address   string        `yaml:"address" env:"XMDM_ADDR"`
	Port      string        `yaml:"port"`
	SessionTTL time.Duration `yaml:"sessionTTL" env:"XMDM_SESSION_TTL"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn" env:"XMDM_POSTGRES_DSN"`
}

type MQTTConfig struct {
	Address              string `yaml:"address" env:"XMDM_MQTT_ADDRESS"`
	ClientID             string `yaml:"clientID" env:"XMDM_MQTT_CLIENT_ID"`
	Username               string `yaml:"username" env:"XMDM_MQTT_USERNAME"`
	Password               string `yaml:"password" env:"XMDM_MQTT_PASSWORD"`
	DynsecAddress         string `yaml:"dynsecAddress" env:"XMDM_MQTT_DYNSEC_ADDRESS"`
	DynsecClientID        string `yaml:"dynsecClientID" env:"XMDM_MQTT_DYNSEC_CLIENT_ID"`
	DynsecAdminUser      string `yaml:"dynsecAdminUser" env:"XMDM_MQTT_DYNSEC_ADMIN_USER"`
	DynsecPassword       string `yaml:"dynsecPassword" env:"XMDM_MQTT_DYNSEC_PASSWORD"`
	KeepAlive            string `yaml:"keepAlive" env:"XMDM_MQTT_KEEPALIVE"`
	DialTimeout          string `yaml:"dialTimeout" env:"XMDM_MQTT_DIAL_TIMEOUT"`
	DynsecKeepAlive      string `yaml:"dynsecKeepAlive" env:"XMDM_MQTT_DYNSEC_KEEPALIVE"`
	DynsecDialTimeout    string `yaml:"dynsecDialTimeout" env:"XMDM_MQTT_DYNSEC_DIAL_TIMEOUT"`
}

type ObjectStoreConfig struct {
	Endpoint        string `yaml:"endpoint" env:"XMDM_OBJECT_STORAGE_ENDPOINT"`
	Region          string `yaml:"region" env:"XMDM_OBJECT_STORAGE_REGION"`
	AccessKeyID      string `yaml:"accessKeyID" env:"XMDM_OBJECT_STORAGE_ACCESS_KEY"`
	SecretAccessKey  string `yaml:"secretAccessKey" env:"XMDM_OBJECT_STORAGE_SECRET_KEY"`
	Bucket           string `yaml:"bucket" env:"XMDM_OBJECT_STORAGE_BUCKET"`
}

type AdminConfig struct {
	Username string `yaml:"username" env:"XMDM_ADMIN_USERNAME"`
	Password string `yaml:"password" env:"XMDM_ADMIN_PASSWORD"`
}

// LoadConfig loads configuration from YAML file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Address: ":8080",
			SessionTTL: 24 * time.Hour,
		},
		Postgres: PostgresConfig{
			DSN: "postgres://xmdm:xmdm@127.0.0.1:5432/xmdm?sslmode=disable",
		},
		MQTT: MQTTConfig{
			Address:              "127.0.0.1:1883",
			ClientID:             "xmdm-server",
			Username:               "xmdm-server",
			Password:               "xmdm-server-secret",
			DynsecAddress:         "127.0.0.1:1883",
			DynsecClientID:        "xmdm-dynsec",
			DynsecAdminUser:      "admin",
			DynsecPassword:       "xmdm-admin",
			KeepAlive:            "30s",
			DialTimeout:          "5s",
			DynsecKeepAlive:      "30s",
			DynsecDialTimeout:    "5s",
		},
		ObjectStore: ObjectStoreConfig{
			Endpoint:        "http://127.0.0.1:8333",
			Region:          "us-east-1",
			AccessKeyID:      "xmdm",
			SecretAccessKey:  "xmdm",
			Bucket:          "xmdm",
		},
		Admin: AdminConfig{
			Username: "admin",
			Password: "admin",
		},
	}

	// Load from YAML file if provided
	if configPath != "" {
		if err := loadFromFile(cfg, configPath); err != nil {
			return nil, err
		}
	}

	// Override with environment variables
	loadFromEnv(cfg)

	return cfg, nil
}

func loadFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, cfg)
}

func loadFromEnv(cfg *Config) {
	// Server config
	if addr := os.Getenv("XMDM_ADDR"); addr != "" {
		cfg.Server.Address = addr
	}
	if sessionTTL := os.Getenv("XMDM_SESSION_TTL"); sessionTTL != "" {
		if dur, err := time.ParseDuration(sessionTTL); err == nil {
			cfg.Server.SessionTTL = dur
		}
	}

	// Postgres config
	if dsn := os.Getenv("XMDM_POSTGRES_DSN"); dsn != "" {
		cfg.Postgres.DSN = dsn
	}

	// Admin config
	if username := os.Getenv("XMDM_ADMIN_USERNAME"); username != "" {
		cfg.Admin.Username = username
	}
	if password := os.Getenv("XMDM_ADMIN_PASSWORD"); password != "" {
		cfg.Admin.Password = password
	}

	// MQTT config
	if mqttAddr := os.Getenv("XMDM_MQTT_ADDRESS"); mqttAddr != "" {
		cfg.MQTT.Address = mqttAddr
	}
	if mqttClientID := os.Getenv("XMDM_MQTT_CLIENT_ID"); mqttClientID != "" {
		cfg.MQTT.ClientID = mqttClientID
	}
	if mqttUsername := os.Getenv("XMDM_MQTT_USERNAME"); mqttUsername != "" {
		cfg.MQTT.Username = mqttUsername
	}
	if mqttPassword := os.Getenv("XMDM_MQTT_PASSWORD"); mqttPassword != "" {
		cfg.MQTT.Password = mqttPassword
	}
	if mqttDynsecAddr := os.Getenv("XMDM_MQTT_DYNSEC_ADDRESS"); mqttDynsecAddr != "" {
		cfg.MQTT.DynsecAddress = mqttDynsecAddr
	}
	if mqttDynsecClientID := os.Getenv("XMDM_MQTT_DYNSEC_CLIENT_ID"); mqttDynsecClientID != "" {
		cfg.MQTT.DynsecClientID = mqttDynsecClientID
	}
	if mqttDynsecUser := os.Getenv("XMDM_MQTT_DYNSEC_ADMIN_USER"); mqttDynsecUser != "" {
		cfg.MQTT.DynsecAdminUser = mqttDynsecUser
	}
	if mqttDynsecPass := os.Getenv("XMDM_MQTT_DYNSEC_PASSWORD"); mqttDynsecPass != "" {
		cfg.MQTT.DynsecPassword = mqttDynsecPass
	}
	if mqttKeepAlive := os.Getenv("XMDM_MQTT_KEEPALIVE"); mqttKeepAlive != "" {
		cfg.MQTT.KeepAlive = mqttKeepAlive
	}
	if mqttDialTimeout := os.Getenv("XMDM_MQTT_DIAL_TIMEOUT"); mqttDialTimeout != "" {
		cfg.MQTT.DialTimeout = mqttDialTimeout
	}
	if mqttDynsecKeepAlive := os.Getenv("XMDM_MQTT_DYNSEC_KEEPALIVE"); mqttDynsecKeepAlive != "" {
		cfg.MQTT.DynsecKeepAlive = mqttDynsecKeepAlive
	}
	if mqttDynsecDialTimeout := os.Getenv("XMDM_MQTT_DYNSEC_DIAL_TIMEOUT"); mqttDynsecDialTimeout != "" {
		cfg.MQTT.DynsecDialTimeout = mqttDynsecDialTimeout
	}

	// Object storage config
	if endpoint := os.Getenv("XMDM_OBJECT_STORAGE_ENDPOINT"); endpoint != "" {
		cfg.ObjectStore.Endpoint = endpoint
	}
	if region := os.Getenv("XMDM_OBJECT_STORAGE_REGION"); region != "" {
		cfg.ObjectStore.Region = region
	}
	if accessKey := os.Getenv("XMDM_OBJECT_STORAGE_ACCESS_KEY"); accessKey != "" {
		cfg.ObjectStore.AccessKeyID = accessKey
	}
	if secretKey := os.Getenv("XMDM_OBJECT_STORAGE_SECRET_KEY"); secretKey != "" {
		cfg.ObjectStore.SecretAccessKey = secretKey
	}
	if bucket := os.Getenv("XMDM_OBJECT_STORAGE_BUCKET"); bucket != "" {
		cfg.ObjectStore.Bucket = bucket
	}
}