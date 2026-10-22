#!/bin/bash
# Container entrypoint: checks the EZVIZ login, writes the MediaMTX config from the environment and starts MediaMTX.
# MediaMTX runs camera.sh for a camera when someone opens its stream, or at startup for ALWAYS_ON cameras.
set -euo pipefail

: "${EZVIZ_EMAIL:?EZVIZ_EMAIL is required}"
: "${EZVIZ_PASSWORD:?EZVIZ_PASSWORD is required}"
: "${CAMERAS:?CAMERAS is required, ex: front-door:BC0123456,garden:BC0654321}"
RECORD="${RECORD:-true}"
RECORD_DELETE_AFTER="${RECORD_DELETE_AFTER:-1d}"
ALWAYS_ON="${ALWAYS_ON:-}"
ALWAYS_ON=",${ALWAYS_ON//[[:space:]]/},"
export TERMINAL_NAME="${TERMINAL_NAME:-LE-EZ-CCTV}"
CONFIG=/tmp/mediamtx.yml

if [[ "$RECORD" != true && "$RECORD" != false ]]; then
	echo "RECORD must be true or false" >&2
	exit 1
fi

paths=""
IFS=',' read -ra entries <<< "$CAMERAS"
for entry in "${entries[@]}"; do
	entry="${entry//[[:space:]]/}"
	[[ -z "$entry" ]] && continue
	name="${entry%%:*}"
	serial="${entry#*:}"
	if [[ "$entry" != *:* || ! "$name" =~ ^[A-Za-z0-9_-]+$ || ! "$serial" =~ ^[A-Za-z0-9]+$ ]]; then
		echo "Invalid CAMERAS entry '$entry', expected name:SERIAL" >&2
		exit 1
	fi
	if [[ "$ALWAYS_ON" == *",$name,"* ]]; then
		paths+="  $name:
    runOnInit: /app/camera.sh $serial
    runOnInitRestart: true
"
		echo "Camera $name ($serial): always on"
	else
		paths+="  $name:
    runOnDemand: /app/camera.sh $serial
    runOnDemandRestart: true
    runOnDemandStartTimeout: 30s
    runOnDemandCloseAfter: 30s
"
		echo "Camera $name ($serial): on demand"
	fi
done

cd /data
mkdir -p recordings

# One login up front: checks the credentials and saves the session (session.json) the camera processes reuse
if ! le-ezviz-vs -preserveSession -terminalName="$TERMINAL_NAME" -logFile=false -logLevel=info; then
	echo "EZVIZ login failed, check EZVIZ_EMAIL and EZVIZ_PASSWORD. Exiting in 5 minutes so restarts don't hammer the login" >&2
	sleep 300
	exit 1
fi

cat > "$CONFIG" <<EOF
logLevel: info
authMethod: internal
authInternalUsers:
  # only the camera processes inside this container may publish
  - user: any
    ips: ["127.0.0.1", "::1"]
    permissions:
      - action: publish
  # anyone may watch
  - user: any
    permissions:
      - action: read
api: false
metrics: false
pprof: false
playback: false
rtsp: true
rtspAddress: 127.0.0.1:8554
rtspTransports: [tcp]
rtmp: false
webrtc: false
srt: false
moq: false
hls: true
hlsAddress: :8888
hlsAllowOrigins: ["*"]
# plain fMP4 HLS: Low-Latency HLS breaks on iOS when part durations drift, which they do with wall clock timestamps
hlsVariant: fmp4
pathDefaults:
  record: $RECORD
  recordPath: /data/recordings/%path/%Y-%m-%d_%H-%M-%S-%f
  recordDeleteAfter: $RECORD_DELETE_AFTER
paths:
$paths
EOF

exec mediamtx "$CONFIG"
