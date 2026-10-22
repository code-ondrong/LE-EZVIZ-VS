#!/bin/bash
# Run by MediaMTX for one camera path (runOnDemand/runOnInit set MTX_PATH and RTSP_PORT): le-ezviz-vs writes the
# camera's video to stdout and ffmpeg publishes it to the path without re-encoding. MediaMTX stops both with SIGINT.
set -o pipefail
cd /data || exit 1
le-ezviz-vs -deviceSerial="$1" -preserveSession -terminalName="${TERMINAL_NAME:-LE-EZ-CCTV}" -output=- -logFile=false -logLevel="${LOG_LEVEL:-info}" |
	ffmpeg -hide_banner -nostdin -loglevel warning \
		-use_wallclock_as_timestamps 1 -probesize 1M -analyzeduration 1M -i pipe:0 \
		-map 0:v:0 -c copy -f rtsp -rtsp_transport tcp "rtsp://127.0.0.1:${RTSP_PORT}/${MTX_PATH}"
