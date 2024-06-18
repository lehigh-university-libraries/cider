#!/usr/bin/env bash

set -eou pipefail

DOCKER_NETWORK="cider-$RANDOM"
docker network create "$DOCKER_NETWORK"

docker run -d \
  --name nginx \
  --network "$DOCKER_NETWORK" \
  --rm \
  nginx:latest

docker build -t cider:latest .

docker run -d \
  --name cider \
  --env BACKEND_HOST=nginx \
  --env REQUEST_THRESHOLD=1 \
  --env REQUEST_WINDOW=1 \
  --rm \
  --network "$DOCKER_NETWORK" \
  -p 8080:8080 \
  cider:latest

STATUS=$(curl -s -w '%{http_code}' -o /dev/null http://127.0.0.1:8080)
echo "$STATUS"
if [ "${STATUS}" != "200" ]; then
  echo "Got non-200 status: $STATUS"
  exit 1
fi

STATUS=$(curl -s -w '%{http_code}' -o /dev/null http://127.0.0.1:8080)
echo "$STATUS"
if [ "${STATUS}" != "302" ]; then
  echo "Got non-302 status: $STATUS"
  exit 1
fi

sleep 2

STATUS=$(curl -s -w '%{http_code}' -o /dev/null http://127.0.0.1:8080)
echo "$STATUS"
if [ "${STATUS}" != "200" ]; then
  echo "Got non-200 status: $STATUS"
  exit 1
fi

docker logs cider
docker stop cider
docker stop nginx
docker network rm "$DOCKER_NETWORK"
