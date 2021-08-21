#!/usr/bin/env bash

cd ../src
go run . -g /aws/containerinsights/dev-EKS/application -p dev -t 3600 -c ../tst/cfg.yaml


