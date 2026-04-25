#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

base_url=${XMDM_BASE_URL:-http://127.0.0.1:8080}
server_url=${XMDM_SERVER_URL:-http://192.168.0.168:8080}
server_project=${XMDM_SERVER_PROJECT:-rest}
device_admin_url=${XMDM_DEVICE_ADMIN_PACKAGE_URL:-$server_url/launcher.apk}
device_admin_checksum=${XMDM_DEVICE_ADMIN_PACKAGE_CHECKSUM:-abc123}
device_id_use=${XMDM_DEVICE_ID_USE:-serial}
adb_serial=${XMDM_ADB_SERIAL:?set XMDM_ADB_SERIAL}
enrollment_token=${XMDM_ENROLLMENT_TOKEN:-}
chrome_apk_path=${XMDM_CHROME_APK_PATH:-$script_dir/../artifacts/chrome.apk}
chrome_apk_name=${XMDM_CHROME_APK_NAME:-chrome.apk}
chrome_apk_storage_key=${XMDM_CHROME_APK_STORAGE_KEY:-artifacts/chrome.apk}
chrome_apk_mime_type=${XMDM_CHROME_APK_MIME_TYPE:-application/vnd.android.package-archive}
chrome_app_package=${XMDM_CHROME_APP_PACKAGE:-com.android.chrome}
chrome_app_name=${XMDM_CHROME_APP_NAME:-Chrome}
chrome_version_name=${XMDM_CHROME_VERSION_NAME:-138.0.7204.179}
chrome_version_code=${XMDM_CHROME_VERSION_CODE:-720417920}
if [ ! -f "$chrome_apk_path" ] && [ -f "$script_dir/../chrome.apk" ]; then
  chrome_apk_path=$script_dir/../chrome.apk
fi

login_cookie=$(mktemp)
cleanup() {
  rm -f "$login_cookie"
}
trap cleanup EXIT

curl -fsS -c "$login_cookie" -d "username=admin&password=admin" "$base_url/api/v1/admin/login" >/dev/null

device_exists_on_server() {
  candidate_device_id=$1
  curl -fsS -b "$login_cookie" "$base_url/api/v1/devices" \
    | python3 -c 'import json, sys
candidate = sys.argv[1]
for record in json.load(sys.stdin):
    if str(record.get("name", "")).strip() == candidate:
        raise SystemExit(0)
raise SystemExit(1)' "$candidate_device_id"
}

if [ -n "${XMDM_DEVICE_ID:-}" ]; then
  device_id=$XMDM_DEVICE_ID
else
  while :; do
    device_id=device-$(date +%s)-$(python3 -c 'import secrets; print(secrets.token_hex(4))')
    if ! device_exists_on_server "$device_id"; then
      break
    fi
  done
fi

adb -s "$adb_serial" shell am force-stop com.xmdm.launcher >/dev/null || true
adb -s "$adb_serial" shell run-as com.xmdm.launcher rm -f files/datastore/agent_state.preferences_pb files/profileInstalled >/dev/null || true
adb -s "$adb_serial" shell run-as com.xmdm.launcher rm -f cache/managed-app-*.apk >/dev/null || true
adb -s "$adb_serial" shell pm uninstall --user 0 com.android.chrome >/dev/null || true

ensure_chrome_apk_uploaded() {
  chrome_file_json=$(
    curl -fsS -b "$login_cookie" "$base_url/api/v1/files" \
      | python3 -c 'import json, sys
storage_key, name = sys.argv[1:3]
for record in json.load(sys.stdin):
    artifact = record.get("artifact") or {}
    if str(record.get("name", "")).strip() == name or str(artifact.get("storageKey", "")).strip() == storage_key:
        print(json.dumps(record, separators=(",", ":")))
        break' "$chrome_apk_storage_key" "$chrome_apk_name"
  )
  if [ -z "$chrome_file_json" ]; then
    if [ ! -f "$chrome_apk_path" ]; then
      echo "missing local chrome apk: $chrome_apk_path" >&2
      exit 1
    fi

    chrome_checksum=$(python3 -c 'import base64, hashlib, pathlib, sys
path = pathlib.Path(sys.argv[1])
data = path.read_bytes()
print(base64.urlsafe_b64encode(hashlib.sha256(data).digest()).decode().rstrip("="))' "$chrome_apk_path")
    chrome_size=$(wc -c < "$chrome_apk_path" | tr -d ' ')

    chrome_file_json=$(
      curl -fsS -b "$login_cookie" \
        -F "name=$chrome_apk_name" \
        -F "storageKey=$chrome_apk_storage_key" \
        -F "checksum=$chrome_checksum" \
        -F "sizeBytes=$chrome_size" \
        -F "mimeType=$chrome_apk_mime_type" \
        -F "file=@$chrome_apk_path;type=$chrome_apk_mime_type" \
        "$base_url/api/v1/files"
    )
  fi

  printf '%s' "$chrome_file_json"
}

ensure_chrome_app_version() {
  chrome_file_json=$1

  chrome_artifact_id=$(printf '%s' "$chrome_file_json" | python3 -c 'import json, sys
record = json.load(sys.stdin)
artifact = record.get("artifact") or {}
print(artifact["id"])')
  chrome_checksum=$(printf '%s' "$chrome_file_json" | python3 -c 'import json, sys
record = json.load(sys.stdin)
artifact = record.get("artifact") or {}
print(artifact.get("checksum") or record["checksum"])')

  chrome_app_json=$(
    curl -fsS -b "$login_cookie" "$base_url/api/v1/apps" \
      | python3 -c 'import json, sys
package_name, app_name = sys.argv[1:3]
for record in json.load(sys.stdin):
    if str(record.get("packageName", "")).strip() == package_name:
        print(json.dumps(record, separators=(",", ":")))
        break' "$chrome_app_package" "$chrome_app_name"
  )
  if [ -z "$chrome_app_json" ]; then
    chrome_app_json=$(
      python3 -c 'import json
print(json.dumps({"packageName":"'"$chrome_app_package"'","name":"'"$chrome_app_name"'"}, separators=(",", ":")))' \
        | curl -fsS -b "$login_cookie" -H 'Content-Type: application/json' --data-binary @- \
          "$base_url/api/v1/apps"
    )
  fi

  chrome_app_id=$(printf '%s' "$chrome_app_json" | python3 -c 'import json, sys
record = json.load(sys.stdin)
print(record["id"])')

  chrome_version_json=$(
    curl -fsS -b "$login_cookie" "$base_url/api/v1/apps/$chrome_app_id/versions" \
      | python3 -c 'import json, sys
version_name, version_code = sys.argv[1:3]
for record in json.load(sys.stdin):
    if str(record.get("versionName", "")).strip() == version_name and str(record.get("versionCode", "")) == version_code and str(record.get("status", "")).strip() == "published":
        print(json.dumps(record, separators=(",", ":")))
        break' "$chrome_version_name" "$chrome_version_code"
  )
  if [ -n "$chrome_version_json" ]; then
    return 0
  fi

  chrome_create_payload=$(
    python3 -c 'import json, sys
artifact_id, checksum, version_name, version_code = sys.argv[1:5]
print(json.dumps({
    "versionName": version_name,
    "versionCode": int(version_code),
    "artifactId": artifact_id,
    "checksum": checksum,
    "publish": True,
}, separators=(",", ":")))' \
      "$chrome_artifact_id" "$chrome_checksum" "$chrome_version_name" "$chrome_version_code"
  )

  if ! chrome_version_json=$(
    printf '%s' "$chrome_create_payload" \
      | curl -fsS -b "$login_cookie" -H 'Content-Type: application/json' --data-binary @- \
        "$base_url/api/v1/apps/$chrome_app_id/versions"
  ); then
    echo "failed to create or publish Chrome version $chrome_version_name/$chrome_version_code for app $chrome_app_id" >&2
    exit 1
  fi

  printf '%s' "$chrome_version_json"
}

chrome_file_json=$(ensure_chrome_apk_uploaded)
ensure_chrome_app_version "$chrome_file_json" >/dev/null

if [ -z "$enrollment_token" ]; then
  enrollment_token=$(curl -fsS -b "$login_cookie" -H 'Content-Type: application/json' -d '{"ttlSeconds":3600}' "$base_url/api/v1/enrollment/tokens" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
fi

qr_request=$(
  python3 -c 'import json, sys
server_url, server_project, enrollment_token, device_id, device_id_use, device_admin_url, device_admin_checksum = sys.argv[1:]
payload = {
    "serverUrl": server_url,
    "serverProject": server_project,
    "enrollmentToken": enrollment_token,
    "deviceAdminPackageDownloadLocation": device_admin_url,
    "deviceAdminPackageChecksum": device_admin_checksum,
    "deviceIdentityPolicy": {
        "deviceId": device_id,
        "deviceIdUse": device_id_use,
    },
    "bootstrapExtras": {
        "CUSTOMER": "Acme",
    },
}
print(json.dumps(payload, separators=(",", ":")))' \
    "$server_url" "$server_project" "$enrollment_token" "$device_id" "$device_id_use" "$device_admin_url" "$device_admin_checksum"
)

qr_json=$(
  printf '%s' "$qr_request" \
    | curl -fsS -b "$login_cookie" -H 'Content-Type: application/json' --data-binary @- \
      "$base_url/api/v1/enrollment/qr/json"
)

bootstrap_uri=$(python3 - "$qr_json" <<'PY'
import base64
import sys

qr_json = sys.argv[1]
encoded = base64.urlsafe_b64encode(qr_json.encode()).decode().rstrip("=")
print("base64url:" + encoded)
PY
)

printf 'enrollment token: %s\n' "$enrollment_token"
printf 'bootstrap uri: %s\n' "$bootstrap_uri"

adb -s "$adb_serial" shell am start -S -n com.xmdm.launcher/.MainActivity --ez com.xmdm.launcher.EXTRA_RESET_STATE true --es com.xmdm.launcher.EXTRA_PROVISIONING_RUN_ID "$device_id" -d "$bootstrap_uri"
