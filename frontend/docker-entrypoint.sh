#!/bin/sh
set -e
cd /frontend

corepack enable
yarn
yarn build
