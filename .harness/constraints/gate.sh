#!/usr/bin/env bash
# litkit 全量门禁 — Linux/macOS
#
# 用法：.harness/constraints/gate.sh
# 等价于 CLAUDE.md 中的完整门禁命令，一步跑完 Go 门禁 + arch-check。
#
# 退出码：
#   0 — 全部通过
#   1 — 任一门禁失败

set -e
root="$(cd "$(dirname "$0")/../.." && pwd)"
app="$root/app"

echo "[1/6] gofmt"
cd "$app"
gofmt -l .

echo "[2/6] golangci-lint"
golangci-lint run --config "$root/.harness/constraints/style/.golangci.yml"

echo "[3/6] go vet"
go vet ./...

echo "[4/6] go test"
go test ./... -cover

echo "[5/6] govulncheck"
govulncheck ./...

echo "[6/6] arch-check"
cd "$root"
go run .harness/constraints/arch/main.go

echo "===== ALL GATES PASSED ====="
