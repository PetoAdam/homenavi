#!/bin/sh
set -e

# The stock eclipse-mosquitto entrypoint recursively chowns /mosquitto/config,
# but our config files are mounted read-only from the workspace. That chown
# attempt spams the logs, so we intentionally skip it while still ensuring the
# data/log directories remain writable by the mosquitto user.
if [ -d /mosquitto/data ]; then
  chown -R mosquitto:mosquitto /mosquitto/data || true
fi
if [ -d /mosquitto/log ]; then
  chown -R mosquitto:mosquitto /mosquitto/log || true
fi

if [ "$#" -gt 0 ]; then
  exec /usr/sbin/mosquitto "$@"
fi

exec /usr/sbin/mosquitto -c /mosquitto/config/mosquitto.conf
