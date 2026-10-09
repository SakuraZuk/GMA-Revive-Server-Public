#!/bin/sh
set -eu
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
export PYTHONHOME="$base/usr"
exec "$base/usr/bin/python2.7" "$@"
