package v1

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appspg "xmdm/server/internal/apps/postgres"
	"xmdm/server/internal/artifacts"
	s3store "xmdm/server/internal/artifacts/s3"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/bootstrap"
	certificatesspg "xmdm/server/internal/certificates/postgres"
	commandspg "xmdm/server/internal/commands/postgres"
	"xmdm/server/internal/config"
	devicepg "xmdm/server/internal/device/postgres"
	deviceinfopg "xmdm/server/internal/deviceinfo/postgres"
	"xmdm/server/internal/enrollment"
	enrollmentpg "xmdm/server/internal/enrollment/postgres"
	filespg "xmdm/server/internal/files/postgres"
	grouppg "xmdm/server/internal/group/postgres"
	identitypg "xmdm/server/internal/identity/postgres"
	logspg "xmdm/server/internal/logs/postgres"
	managedfilespg "xmdm/server/internal/managedfiles/postgres"
	"xmdm/server/internal/mqttdynsec"
	"xmdm/server/internal/plugins"
	policypg "xmdm/server/internal/policy/postgres"
	"xmdm/server/internal/push"
	telemetrypg "xmdm/server/internal/telemetry/postgres"
)

// NewDeps initializes all dependencies for the API layer
func NewDeps(cfg *config.Config) Dependencies {
	dsn := cfg.Postgres.DSN
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	provisioner := mustMQTTProvisioner(cfg)
	publisherUsername := cfg.MQTT.Username
	publisherPassword := cfg.MQTT.Password
	if err := ensureServerPublisherWithRetry(context.Background(), provisioner, publisherUsername, publisherPassword); err != nil {
		log.Printf("mqtt dynsec server publisher provisioning failed: %v", err)
	}
	artifactStore := mustArtifactStore(cfg)
	pushPublisher := mustPushPublisher(cfg)
	devicesStore := devicepg.New(pool)
	deviceInfoStore := deviceinfopg.New(pool)
	enrollmentStore := enrollmentpg.New(pool)
	commandStore := commandspg.New(pool)
	commandStore.SetPublisher(pushPublisher)
	devicesStore.SetProvisioner(provisioner)
	enrollmentStore.SetProvisioner(provisioner)
	return Dependencies{
		Database:     pool,
		Identity:     identitypg.New(pool),
		Apps:         appspg.New(pool),
		Files:        filespg.New(pool),
		ManagedFiles: managedfilespg.New(pool),
		Logs:         logspg.New(pool),
		Commands:     commandStore,
		DeviceInfo:   deviceInfoStore,
		Certificates: certificatesspg.New(pool),
		Artifacts:    artifactStore,
		Groups:       grouppg.New(pool),
		Policies:     policypg.New(pool),
		Devices:      devicesStore,
		Enrollment:   enrollmentStore,
		Telemetry:    telemetrypg.New(pool),
		Audit:        auditpg.NewDBStore(pool),
		Push:         pushPublisher,
		Runtime: enrollment.RuntimeSnapshot{
			MqttAddress:           cfg.MQTT.Address,
			CommandPollIntervalMs: cfg.Device.CommandPollInterval.Milliseconds(),
			ConfigSyncIntervalMs:  cfg.Device.ConfigSyncInterval.Milliseconds(),
		},
		ServerPublicURL: cfg.Server.PublicURL,
		AgentAppPackage: cfg.Device.AgentAppPackage,
		TenantID:        bootstrap.SeedTenantID,
		PluginManager:   plugins.Disabled(),
	}
}

func ensureServerPublisherWithRetry(ctx context.Context, provisioner mqttdynsec.Provisioner, username, password string) error {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if err := provisioner.EnsureServerPublisher(ctx, username, password); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}

func mustMQTTProvisioner(cfg *config.Config) mqttdynsec.Provisioner {
	keepAlive, err := time.ParseDuration(cfg.MQTT.DynsecKeepAlive)
	if err != nil {
		keepAlive = 30 * time.Second
	}

	dialTimeout, err := time.ParseDuration(cfg.MQTT.DynsecDialTimeout)
	if err != nil {
		dialTimeout = 5 * time.Second
	}

	provisioner, err := mqttdynsec.New(mqttdynsec.Config{
		Address:     cfg.MQTT.DynsecAddress,
		ClientID:    cfg.MQTT.DynsecClientID,
		Username:    cfg.MQTT.DynsecAdminUser,
		Password:    cfg.MQTT.DynsecPassword,
		KeepAlive:   keepAlive,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		log.Fatalf("init mqtt dynsec provisioner: %v", err)
	}
	return provisioner
}

func mustPushPublisher(cfg *config.Config) push.Publisher {
	keepAlive, err := time.ParseDuration(cfg.MQTT.KeepAlive)
	if err != nil {
		keepAlive = 30 * time.Second
	}

	timeout, err := time.ParseDuration(cfg.MQTT.DialTimeout)
	if err != nil {
		timeout = 5 * time.Second
	}

	pub, err := push.NewMQTTPublisher(push.MQTTConfig{
		Address:     cfg.MQTT.Address,
		ClientID:    cfg.MQTT.ClientID,
		Username:    cfg.MQTT.Username,
		Password:    cfg.MQTT.Password,
		KeepAlive:   keepAlive,
		DialTimeout: timeout,
	})
	if err != nil {
		log.Fatalf("init mqtt publisher: %v", err)
	}
	return pub
}

func mustArtifactStore(cfg *config.Config) artifacts.Store {
	store, err := s3store.New(context.Background(), s3store.Config{
		Endpoint:        cfg.ObjectStore.Endpoint,
		Region:          cfg.ObjectStore.Region,
		AccessKeyID:     cfg.ObjectStore.AccessKeyID,
		SecretAccessKey: cfg.ObjectStore.SecretAccessKey,
		Bucket:          cfg.ObjectStore.Bucket,
		UsePathStyle:    true,
	})
	if err != nil {
		log.Fatalf("init object storage: %v", err)
	}
	return store
}
