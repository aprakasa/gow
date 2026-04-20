#!/usr/bin/env bash
# smoke.sh — build the binary in a container and verify basic commands work.
# Run: ./test/smoke.sh
set -euo pipefail

tag="gow-smoke-$RANDOM"
image="gow-smoke:$tag"
container="smoke-$tag"
trap "docker rm -f $container > /dev/null 2>&1 || true; docker rmi $image > /dev/null 2>&1 || true" EXIT

echo "==> Building smoke image..."
docker build -f Dockerfile.smoke -t "$image" .

echo "==> Running smoke tests..."
docker run --name "$container" "$image" bash -c '
set -e
errors=0

# Binary exists and is executable.
gow --help > /dev/null || { echo "FAIL: gow --help"; errors=$((errors+1)); }

# Subcommands are registered.
for cmd in site stack presets reconcile status; do
    gow "$cmd" --help > /dev/null || { echo "FAIL: gow $cmd --help"; errors=$((errors+1)); }
done

# Site subcommands.
for cmd in create update info list online offline delete ssl; do
    gow site "$cmd" --help > /dev/null || { echo "FAIL: gow site $cmd --help"; errors=$((errors+1)); }
done

# Stack subcommands.
for cmd in install upgrade remove purge start stop restart reload status; do
    gow stack "$cmd" --help > /dev/null || { echo "FAIL: gow stack $cmd --help"; errors=$((errors+1)); }
done

gow presets > /dev/null || { echo "FAIL: gow presets"; errors=$((errors+1)); }

if [ $errors -gt 0 ]; then
    echo "FAILED: $errors smoke test(s)"
    exit 1
fi
echo "All smoke tests passed."
'

echo "==> Smoke tests OK"
