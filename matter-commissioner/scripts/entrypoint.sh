#!/bin/sh

set -eu

mkdir -p /run/dbus /run/avahi-daemon /var/lib/matter-commissioner/chip-tool

started_local_dbus=0

if [ ! -S /run/dbus/system_bus_socket ]; then
	dbus-daemon --system --fork
	started_local_dbus=1
fi

if [ "$started_local_dbus" -eq 1 ] && [ ! -S /run/avahi-daemon/socket ]; then
	avahi-daemon --daemonize --no-chroot
fi

exec /app/matter-commissioner