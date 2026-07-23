#!/bin/sh
set -eu
mkdir -p /data
chown -R flaredns:flaredns /data
exec su-exec flaredns "$@"
