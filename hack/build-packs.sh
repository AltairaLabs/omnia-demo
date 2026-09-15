#!/usr/bin/env bash
set -euo pipefail

# Compile the public Arena projects into local-only artifacts. The generated
# JSON is intentionally kept under build/ and is not committed: a released
# Omnia chart or operator should own the deployment packaging and versioning.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKC="${PACKC:-packc}"
OUT="${ROOT}/build/packs"
mkdir -p "${OUT}"

for demo in mortgage-underwriting pii-chat claims-triage sharepoint-adapter; do
  "${PACKC}" compile \
    --config "${ROOT}/demos/${demo}/arena/config.arena.yaml" \
    --output "${OUT}/${demo}.pack.json" \
    --id "${demo}"
  "${PACKC}" validate "${OUT}/${demo}.pack.json"
done

echo "Compiled public packs into ${OUT}"
