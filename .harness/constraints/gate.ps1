# litkit 全量门禁 — Windows PowerShell
#
# 用法：.harness/constraints/gate.ps1
# 一步跑完：gofmt → build → lint → vet → test → arch-check → sync。
# 构建产出 app/litkit.exe（gitignored）；触网测试不进 gate（手动跑）。
#
# 退出码：
#   0 — 全部通过
#   1 — 任一门禁失败

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/../.."
$app = Join-Path $root "app"

Write-Host "[1/7] gofmt" -ForegroundColor Cyan
Push-Location $app
$unformatted = gofmt -l .
if ($unformatted) { Write-Host "FAIL: gofmt`n$unformatted" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[2/7] go build" -ForegroundColor Cyan
go build -o litkit.exe ./cmd/litkit
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: go build" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[3/7] golangci-lint" -ForegroundColor Cyan
golangci-lint run ./... --config (Join-Path $root ".harness/constraints/style/.golangci.yml")
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: golangci-lint" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[4/7] go vet" -ForegroundColor Cyan
go vet ./...
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: go vet" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[5/7] go test + coverage" -ForegroundColor Cyan
go test ./... '-coverprofile=coverage.out'
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: go test" -ForegroundColor Red; Pop-Location; exit 1 }
$covOut = go tool cover '-func=coverage.out'
$covLine = ($covOut | Select-String "^total:").ToString()
$pct = [float]([regex]::Match($covLine, '([0-9.]+)%').Groups[1].Value)
if ($pct -lt 60) { Write-Host "FAIL: coverage $pct% < 60%" -ForegroundColor Red; Pop-Location; exit 1 }
Remove-Item coverage.out -ErrorAction SilentlyContinue

Pop-Location

Write-Host "[6/7] arch-check" -ForegroundColor Cyan
Push-Location $root
go run .harness/constraints/arch/main.go
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: arch-check" -ForegroundColor Red; Pop-Location; exit 1 }

Write-Host "[7/7] sync (CLI/api.md 一致性 + 断链)" -ForegroundColor Cyan
Push-Location (Join-Path $root ".harness/constraints/sync")
go run .
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: sync" -ForegroundColor Red; Pop-Location; exit 1 }
Pop-Location
Pop-Location

Write-Host "===== ALL GATES PASSED =====" -ForegroundColor Green
