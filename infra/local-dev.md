# Local Development

## Requirements

- Docker Engine and Docker Compose
- Go toolchain for the server
- Android Studio or the Android command-line toolchain
- A provisionable Android emulator or physical device for agent testing

## Local Stack

Bring up the supporting services first:

```sh
cd infra
docker compose up -d
./migrate.sh
```

The local stack provides:

- PostgreSQL
- SeaweedFS S3-compatible object storage
- MQTT broker

## Bootstrap Flow

1. Start the local stack.
2. Apply the database migrations and seed data.
3. Run the server with the local configuration file.
4. Point the Android agent at the local server URL.
5. Enroll a device and verify sync.

## Notes

- Keep local credentials separate from production credentials.
- The compose file pins image tags so the local environment does not drift unexpectedly.
- SeaweedFS exposes an S3 endpoint on `localhost:8333` and accepts arbitrary access and secret keys by default, so use a local-only credential pair in app and server config.
- If a service container is stale, stop the stack and recreate it before debugging the agent.
