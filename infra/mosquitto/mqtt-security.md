# MQTT Security Contract

XMDM uses MQTT for push delivery.

Topic contract:

- Server publishes device commands to `devices/{deviceId}/commands`.
- Device clients subscribe only to `devices/{deviceId}/commands` for their own `deviceId`.

Broker contract:

- Device clients authenticate with a username that matches their device ID.
- The server uses the dynsec admin client `admin`.
- The command publisher uses the `xmdm-server` client.
- Mosquitto dynamic-security roles must deny cross-device reads and writes.
- Anonymous access must remain disabled when dynamic security is enabled.

Operational note:

- The server updates broker clients and roles through the MQTT topic API at `$CONTROL/dynamic-security/v1`.
- The broker config itself stays stable; only auth state changes per device enrollment.
