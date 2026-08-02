#!/usr/bin/env bash
# litkit 全量门禁 — Linux/macOS
#
# 用法：.harness/constraints/gate.sh
# 一步跑完：gofmt → build → lint → vet → test → vulncheck → arch-check。
# 构建产出 app/litkit（gitignored）；触网测试不进 gate（手动跑）。
#
# 退出码：
#   0 — 全部通过
#   1 — 任一门禁失败

set -e
root="$(cd "$(dirname "$0")/../.." && pwd)"
app="$root/app"

echo "[1/7] gofmt"
cd "$app"
gofmt -l .

echo "[2/7] go build"
go build -o litkit ./cmd/litkit

echo "[3/7] golangci-lint"
golangci-lint run --config "$root/.harness/constraints/style/.golangci.yml"

echo "[4/7] go vet"
go vet ./...

echo "[5/7] go test"
go test ./... -cover

echo "[6/7] govulncheck"
govulncheck ./...

echo "[7/7] arch-check"
cd "$root"
go run .harness/constraints/arch/main.go

echo "===== ALL GATES PASSED ====="
