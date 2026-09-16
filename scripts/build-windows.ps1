param(
    [string]$Version = "dev",
    [string]$Output = "hostrans.exe"
)

$root = Split-Path -Parent $PSScriptRoot
$secretsPath = Join-Path $root ".secrets"
if (-not (Test-Path -LiteralPath $secretsPath)) {
    throw "missing .secrets"
}

$values = @{}
foreach ($line in Get-Content -LiteralPath $secretsPath) {
    $line = $line.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) {
        continue
    }
    if ($line -match "^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$") {
        $value = $Matches[2].Trim()
        if ($value.Length -ge 2 -and (
            ($value.StartsWith('"') -and $value.EndsWith('"')) -or
            ($value.StartsWith("'") -and $value.EndsWith("'"))
        )) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        $values[$Matches[1]] = $value
    }
}

$key = $values["HOSTTRANS_AI_KEY"]
if ([string]::IsNullOrWhiteSpace($key)) {
    throw "missing HOSTTRANS_AI_KEY in .secrets"
}

$base = $values["HOSTTRANS_AI_BASE"]
if ([string]::IsNullOrWhiteSpace($base)) {
    $base = "https://gateway.ai.cloudflare.com/v1/64d53ca476db7004bc2b51e1d9db2dad/translation/compat"
}
$model = $values["HOSTTRANS_AI_MODEL"]
if ([string]::IsNullOrWhiteSpace($model)) {
    $model = "dynamic/free"
}

$ldflags = @(
    "-s",
    "-w",
    "-H",
    "windowsgui",
    "-X",
    "hostrans/dlog.Version=$Version",
    "-X",
    "hostrans/translator.AIKey=$key",
    "-X",
    "hostrans/translator.AIBase=$base",
    "-X",
    "hostrans/translator.AIModel=$model"
) -join " "

Push-Location $root
try {
    & go build "-ldflags=$ldflags" -o $Output .
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
} finally {
    Pop-Location
}

Write-Output "built $Output version=$Version model=$model"
