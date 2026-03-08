#!/bin/sh
# Ensure volume is writable by node (may be root-owned from previous run)
chown -R node:node /home/node/.openclaw 2>/dev/null || true
exec gosu node "$@"
