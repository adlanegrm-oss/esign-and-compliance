# ============================================================
#  Tests d'authentification - Endpoint /api/v1/admin/metrics
#  Projet : esign-and-compliance
# ============================================================

param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$AdminToken = "YuaaGH8vG5P7wsBmJ2hoJnGxS2E85_sz62asKdxgLEY"
)

$uri = "$BaseUrl/api/v1/admin/metrics"

Write-Host ""
Write-Host "=== Tests d'authentification Admin Metrics ===" -ForegroundColor Cyan
Write-Host "URL testee : $uri"
Write-Host ""

$tests = @(
    @{ Nom = "Sans jeton";              Headers = @{}; Attendu = 401 },
    @{ Nom = "Jeton arbitraire";        Headers = @{ Authorization = "Bearer abc123xyz" }; Attendu = 401 },
    @{ Nom = "Jeton faux";              Headers = @{ Authorization = "Bearer faux.jeton.invalide" }; Attendu = 401 },
    @{ Nom = "Mauvais schema";          Headers = @{ Authorization = "Basic abc123" }; Attendu = 401 },
    @{ Nom = "Jeton configure Docker";  Headers = @{ Authorization = "Bearer $AdminToken" }; Attendu = 200 }
)

$echecs = 0

foreach ($test in $tests) {
    try {
        $r = Invoke-WebRequest -UseBasicParsing -Uri $uri -Headers $test.Headers -Method Get -ErrorAction Stop
        $code = [int]$r.StatusCode
    }
    catch {
        if ($null -ne $_.Exception.Response) {
            $code = [int]$_.Exception.Response.StatusCode
        }
        else {
            $code = 0
            Write-Host "$($test.Nom) : Impossible de joindre le serveur - ECHEC" -ForegroundColor Red
            $echecs++
            continue
        }
    }

    if ($code -eq $test.Attendu) {
        Write-Host "$($test.Nom) : HTTP $code - OK" -ForegroundColor Green
    }
    else {
        Write-Host "$($test.Nom) : HTTP $code, attendu $($test.Attendu) - ECHEC" -ForegroundColor Red
        $echecs++
    }
}

Write-Host ""
if ($echecs -eq 0) {
    Write-Host "[SUCCES TOTAL] Tous les tests conteneurises sont passes avec succes !" -ForegroundColor Green
    exit 0
}
else {
    Write-Host "$echecs test(s) en echec." -ForegroundColor Red
    exit 1
}
