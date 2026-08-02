# litkit 全量门禁 — Windows PowerShell
#
# 用法：.harness/constraints/gate.ps1
# 等价于 CLAUDE.md 中的完整门禁命令，一步跑完 Go 门禁 + arch-check。
#
# 退出码：
#   0 — 全部通过
#   1 — 任一门禁失败

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/../.."
$app = Join-Path $root "app"

Write-Host "[1/6] gofmt" -ForegroundColor Cyan
Push-Location $app
gofmt -l .
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: gofmt" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[2/6] golangci-lint" -ForegroundColor Cyan
golangci-lint run --config (Join-Path $root ".harness/constraints/style/.golangci.yml")
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: golangci-lint" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[3/6] go vet" -ForegroundColor Cyan
go vet ./...
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: go vet" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[4/6] go test" -ForegroundColor Cyan
go test ./... -cover
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: go test" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[5/6] govulncheck" -ForegroundColor Cyan
govulncheck ./...
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: govulncheck" -ForegroundColor Red; Pop-Location; exit 1 }
Pop-Location

Write-Host "[6/6] arch-check" -ForegroundColor Cyan
Push-Location $root
go run .harness/constraints/arch/main.go
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: arch-check" -ForegroundColor Red; Pop-Location; exit 1 }
Pop-Location

Write-Host "===== ALL GATES PASSED =====" -ForegroundColor Green
