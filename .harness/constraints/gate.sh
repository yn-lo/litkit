#!/usr/bin/env bash
# litkit 全量门禁 — Linux/macOS
#
# 用法：.harness/constraints/gate.sh
# 一步跑完：gofmt → build → lint → vet → test → arch-check → sync。
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
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then echo "FAIL: gofmt"; echo "$unformatted"; exit 1; fi

echo "[2/7] go build"
go build -o litkit ./cmd/litkit

echo "[3/7] golangci-lint"
golangci-lint run ./... --config "$root/.harness/constraints/style/.golangci.yml"

echo "[4/7] go vet"
go vet ./...

echo "[5/7] go test + coverage"
go test ./... -coverprofile=coverage.out
pct=$(go tool cover -func=coverage.out | awk '/^total:/{gsub(/%/,"",$NF); print $NF}')
if [ -z "$pct" ]; then echo "FAIL: coverage parse"; exit 1; fi
awk "BEGIN{exit ($pct < 60)}" || { echo "FAIL: coverage $pct% < 60%"; exit 1; }
rm -f coverage.out

echo "[6/7] arch-check"
cd "$root"
go run .harness/constraints/arch/main.go

echo "[7/7] sync"
cd "$root/.harness/constraints/sync"
go run .

echo "===== ALL GATES PASSED ====="
