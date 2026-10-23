#!/bin/bash
# Run by MediaMTX for one camera path (runOnDemand/runOnInit set MTX_PATH and RTSP_PORT): le-ezviz-vs publishes the
# camera's video to the path, RTP video directly with the camera's timestamps and MPEG-PS through ffmpeg.
# MediaMTX stops it with SIGINT.
# Usage: camera.sh SERIAL [STREAM], STREAM is 1 for the main stream (default) or 2 for the sub stream
cd /data || exit 1
exec le-ezviz-vs -deviceSerial="$1" -stream="${2:-1}" -preserveSession -terminalName="${TERMINAL_NAME:-LE-EZ-CCTV}" \
	-publish="rtsp://127.0.0.1:${RTSP_PORT}/${MTX_PATH}" -logFile=false -logLevel="${LOG_LEVEL:-info}"
