#!/usr/bin/env bash
set -euo pipefail
N=${1:-100}
for i in $(seq 1 "$N"); do
  go run ./cmd/client "echo job-$i && sleep 0.1" >/dev/null &
done
wait
echo "submitted $N jobs"
