#!/bin/sh
set -eu

curl --fail-with-body --silent --show-error \
  -X POST http://localhost:8080/workspace/activity \
  -H 'Content-Type: application/json' \
  -d '{"workspace":"compiler-tools","developer_id":"dev-7","operation":"build","status":"succeeded","revision":"a1b2c3d","request_id":"ci-1842"}'
printf '\n'
