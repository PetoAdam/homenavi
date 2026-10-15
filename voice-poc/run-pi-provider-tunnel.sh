#!/usr/bin/env sh
set -eu

PI_HOST=${PI_HOST:-ubuntu@192.168.1.231}
LOCAL_STT_PORT=${LOCAL_STT_PORT:-18001}
LOCAL_TTS_PORT=${LOCAL_TTS_PORT:-18002}

exec ssh \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  -N \
  -R "${LOCAL_STT_PORT}:127.0.0.1:${LOCAL_STT_PORT}" \
  -R "${LOCAL_TTS_PORT}:127.0.0.1:${LOCAL_TTS_PORT}" \
  "$PI_HOST"