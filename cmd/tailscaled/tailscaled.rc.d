#!/bin/sh
# $NetBSD: tailscaled.rc.d,v 1.0 2024/01/01 00:00:00 tailscale Exp $
#
# PROVIDE: tailscaled
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="tailscaled"
rcvar=${name}
command="/usr/sbin/tailscaled"
command_args="--state=/var/lib/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock"
pidfile="/var/run/tailscale/${name}.pid"
start_precmd="tailscaled_prestart"
stop_postcmd="tailscaled_poststop"

tailscaled_prestart()
{
	# Create required directories
	mkdir -p /var/lib/tailscale
	mkdir -p /var/run/tailscale
	chmod 700 /var/lib/tailscale
	chmod 755 /var/run/tailscale
	
	# Clean up any existing state
	${command} --cleanup 2>/dev/null || true
}

tailscaled_poststop()
{
	# Clean up on stop
	${command} --cleanup 2>/dev/null || true
}

load_rc_config $name
run_rc_command "$1"
