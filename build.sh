#!/usr/bin/env bash
# Hestia macOS / Linux 构建脚本
set -e
cd "$(dirname "$0")"
mkdir -p dist
OUT=dist/Hestia
[ "$(uname -s)" = "Darwin" ] && OUT=dist/Hestia.app/Contents/MacOS/Hestia && mkdir -p "$(dirname "$OUT")"
go build -trimpath -ldflags "-s -w" -o "$OUT" .
echo "构建完成: $OUT"
echo "直接运行即可（配置与数据保存在可执行文件同目录）"
