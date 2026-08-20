param(
    [string]$Dir = "migrations",
    [string]$URL = $env:CRDB_URL
)

if (-not $URL) {
    Write-Error "CRDB_URL is not set. Pass -URL or set the CRDB_URL env var."
    exit 1
}

$env:CRDB_URL = $URL
go run ./cmd/stillmind migrate -dir $Dir
