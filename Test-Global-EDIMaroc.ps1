
[CmdletBinding()]
param(
    [ValidateSet("Recette", "Preprod", "Prod")]
    [string]$Environment = "Recette",

    [string]$BaseUrl
)

$ErrorActionPreference = "Stop"

# Localiser le fichier de secrets sans afficher son contenu.
$envFileName = ".env.$($Environment.ToLowerInvariant())"

$envCandidates = @(
    (Join-Path $PSScriptRoot $envFileName),
    (Join-Path (Join-Path $PSScriptRoot "archive_pocs\esign-and-compliance-integration") $envFileName)
)

$envFile = $envCandidates |
    Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } |
    Select-Object -First 1

if (-not $envFile) {
    throw "Fichier d'environnement introuvable : $envFileName"
}

# Charger le token depuis le fichier sélectionné.
$tokenLine = Get-Content -LiteralPath $envFile |
    Where-Object { $_ -match '^ADMIN_API_TOKEN=' } |
    Select-Object -First 1

if (-not $tokenLine) {
    throw "ADMIN_API_TOKEN absent de $envFileName"
}

$AdminToken = $tokenLine.Substring("ADMIN_API_TOKEN=".Length).Trim()

if ([string]::IsNullOrWhiteSpace($AdminToken) -or $AdminToken -eq "REMPLACER") {
    throw "Le token de $envFileName est vide ou n'a pas été configuré."
}

# URL par défaut : point d'entrée HTTP du proxy local.
# -BaseUrl permet de tester directement le backend ou une autre URL.
if ([string]::IsNullOrWhiteSpace($BaseUrl)) {
    switch ($Environment) {
        "Recette" { $BaseUrl = "http://127.0.0.1:18081" }
        "Preprod" { $BaseUrl = "http://127.0.0.1:28081" }
        "Prod"    { $BaseUrl = "http://127.0.0.1:38081" }
    }
}

$BaseUrl = $BaseUrl.TrimEnd("/")

$script:TestResults = @()

function Log-Test {
    param(
        [string]$Zone,
        [string]$Name,
        [string]$Status,
        [long]$DurationMs,
        [string]$Details = ""
    )

    $color = switch ($Status) {
        "OK"   { "Green" }
        "FAIL" { "Red" }
        default { "Yellow" }
    }

    Write-Host "[$Status] [$Zone] $Name (${DurationMs}ms) $Details" -ForegroundColor $color

    $script:TestResults += [PSCustomObject]@{
        Zone       = $Zone
        Name       = $Name
        Status     = $Status
        DurationMs = $DurationMs
        Details    = $Details
    }
}

function Invoke-TestRequest {
    param(
        [string]$Zone,
        [string]$Name,
        [string]$Endpoint,
        [hashtable]$Headers = @{},
        [int[]]$Expected = @(200)
    )

    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    $code = 0

    try {
        $uri = "$BaseUrl$Endpoint"

        if ($Headers.Count -gt 0) {
            $response = Invoke-WebRequest `
                -Uri $uri `
                -Method Get `
                -Headers $Headers `
                -UseBasicParsing `
                -ErrorAction Stop
        }
        else {
            $response = Invoke-WebRequest `
                -Uri $uri `
                -Method Get `
                -UseBasicParsing `
                -ErrorAction Stop
        }

        $code = [int]$response.StatusCode
    }
    catch {
        if ($_.Exception.Response) {
            try {
                $code = [int]$_.Exception.Response.StatusCode
            }
            catch {
                $code = 0
            }
        }
        else {
            $code = 0
        }
    }
    finally {
        $stopwatch.Stop()
    }

    $ok = $Expected -contains $code
    $status = if ($ok) { "OK" } else { "FAIL" }
    $expectedText = $Expected -join "/"
    $details = "(HTTP $code, attendu $expectedText)"

    Log-Test -Zone $Zone `
        -Name $Name `
        -Status $status `
        -DurationMs $stopwatch.ElapsedMilliseconds `
        -Details $details
}

Write-Host "========================================" -ForegroundColor Cyan
Write-Host " EDIMAROC - TESTS DE SÉCURITÉ" -ForegroundColor Cyan
Write-Host " Environnement : $Environment" -ForegroundColor Cyan
Write-Host " URL : $BaseUrl" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# Utilisateurs : authentification et rejet des accès non autorisés.
Invoke-TestRequest `
    -Zone "Auth" `
    -Name "1. Sans token (/api/v1/users)" `
    -Endpoint "/api/v1/users" `
    -Expected @(401)

Invoke-TestRequest `
    -Zone "Auth" `
    -Name "2. Token invalide (/api/v1/users)" `
    -Endpoint "/api/v1/users" `
    -Headers @{ Authorization = "Bearer token-invalide" } `
    -Expected @(401)

Invoke-TestRequest `
    -Zone "Auth" `
    -Name "3. Token valide (/api/v1/users)" `
    -Endpoint "/api/v1/users" `
    -Headers @{ Authorization = "Bearer $AdminToken" } `
    -Expected @(200)

# Métriques : vérification indépendante de la route administrative.
Invoke-TestRequest `
    -Zone "Metrics" `
    -Name "4. Sans token (/api/v1/admin/metrics)" `
    -Endpoint "/api/v1/admin/metrics" `
    -Expected @(401)

Invoke-TestRequest `
    -Zone "Metrics" `
    -Name "5. Token invalide (/api/v1/admin/metrics)" `
    -Endpoint "/api/v1/admin/metrics" `
    -Headers @{ Authorization = "Bearer token-invalide" } `
    -Expected @(401)

Invoke-TestRequest `
    -Zone "Metrics" `
    -Name "6. Token valide (/api/v1/admin/metrics)" `
    -Endpoint "/api/v1/admin/metrics" `
    -Headers @{ Authorization = "Bearer $AdminToken" } `
    -Expected @(200)

$passed = @($script:TestResults | Where-Object { $_.Status -eq "OK" }).Count
$failed = @($script:TestResults | Where-Object { $_.Status -eq "FAIL" }).Count
$total = $script:TestResults.Count

Write-Host ""
Write-Host "Résultat : $passed réussis, $failed échecs sur $total tests." -ForegroundColor Cyan

Remove-Variable AdminToken, tokenLine -ErrorAction SilentlyContinue
