#!/usr/bin/env bash
set -e

build() {
  local dir=$1
  local tag=$2
  echo "==> Building $tag from $dir"
  (cd "$dir" && docker build -t "$tag" .)
}

build . tsql
build agent_proxy agent_proxy
build langgraph tsql-langgraph
