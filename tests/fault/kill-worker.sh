#!/usr/bin/env bash
set -euo pipefail
id=$(docker compose ps -q worker | head -n1)
[ -n "$id" ] && docker kill "$id"
echo "killed worker $id; expired leases are requeued by the coordinator"
