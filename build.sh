#!/usr/bin/env bash

IMG="sgaunet/awslogcheck:latest"

#docker image rm "$IMG"
docker build --no-cache . -t "$IMG"
rc=$?

if [ "$rc" != "0" ]
then
  echo "Build FAILED"
  exit 1
fi

docker push "$IMG"