#!/usr/bin/env bash
# 把标签（v0.6.0）换成版本名和版本号，给手机 App 用：
#   name=0.6.0
#   code=600     （主版本 × 10000 + 次版本 × 100 + 修订号，最小为 1）
# 没有标签时（PR 构建）是 0.0.0-dev 和 1。
set -euo pipefail
v="${1:-}"
v="${v#v}"
[ -z "$v" ] && v="0.0.0-dev"
IFS=. read -r major minor patch <<<"${v%%-*}"
code=$((10#${major:-0} * 10000 + 10#${minor:-0} * 100 + 10#${patch:-0}))
[ "$code" -lt 1 ] && code=1
echo "name=$v"
echo "code=$code"
