#!/usr/bin/env bash

sleep 30

output=$(./vcr-cli deploy -z /app/tests/integration/testdata/test.tar.gz  --config-file /app/tests/integration/testdata/config.yaml -f /app/tests/integration/testdata/vcr.yaml)

if echo "$output" | grep -q "Instance has been deployed!"; then
    echo "Success detected in output."
    exit 0
elif echo "$output" | grep -q "failed"; then
    echo "Error detected in output."
    exit 1
else
    echo "No specific success or error message detected."
    exit 2
fi