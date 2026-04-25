#!/usr/bin/env sh
set -eu

admin_password=${MOSQUITTO_DYNSEC_PASSWORD:-xmdm-admin}

plugin_path=""
for candidate in /usr/lib/mosquitto_dynamic_security.so /usr/lib/x86_64-linux-gnu/mosquitto_dynamic_security.so; do
	if [ -f "$candidate" ]; then
		plugin_path=$candidate
		break
	fi
done

if [ -z "$plugin_path" ]; then
	echo "mosquitto dynamic-security plugin not found" >&2
	exit 1
fi

mkdir -p /mosquitto/config /mosquitto/data
printf '%s\n' "$admin_password" > /mosquitto/config/dynsec-password.txt

cat > /mosquitto/config/mosquitto.conf <<EOF
listener 1883
allow_anonymous false
per_listener_settings false
plugin $plugin_path
plugin_opt_config_file /mosquitto/data/dynamic-security.json
plugin_opt_password_init_file /mosquitto/config/dynsec-password.txt
EOF

exec mosquitto -c /mosquitto/config/mosquitto.conf
