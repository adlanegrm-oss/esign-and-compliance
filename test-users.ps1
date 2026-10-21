$ErrorActionPreference = "Continue"

Write-Host "`n=== Tests des Endpoints Utilisateurs ===" -ForegroundColor Cyan

$baseUrl = "http://localhost:8080"
$secureToken = Read-Host "Jeton administrateur" -AsSecureString
$adminToken = [System.Net.NetworkCredential]::new("", $secureToken).Password

$tests = @(
    @{
        Nom = "Liste sans jeton"
        Method = "Get"
        Uri = "$baseUrl/api/v1/users"
        Headers = @{}
        Body = $null
        Attendu = 401
    },
    @{
        Nom = "Liste avec jeton administrateur"
        Method = "Get"
        Uri = "$baseUrl/api/v1/users"
        Headers = @{ Authorization = "Bearer $adminToken" }
        Body = $null
        Attendu = 200
    },
    @{
        Nom = "Création d'un utilisateur"
        Method = "Post"
        Uri = "$baseUrl/api/v1/users"
        Headers = @{
            Authorization = "Bearer $adminToken"
            "Content-Type" = "application/json"
        }
        Body = @{
            username = "nouveau_client"
            email = "client@maroc.ma"
            role = "ADHERENT"
        } | ConvertTo-Json
        Attendu = 201
    }
)

$echecs = 0

foreach ($test in $tests) {
    Write-Host "`nTest : $($test.Nom)" -ForegroundColor Yellow

    $params = @{
        Uri = $test.Uri
        Method = $test.Method
        Headers = $test.Headers
        UseBasicParsing = $true
        ErrorAction = "Stop"
    }

    if ($null -ne $test.Body) {
        $params.Body = $test.Body
        $params.ContentType = "application/json"
    }

    try {
        $r = Invoke-WebRequest @params
        $code = [int]$r.StatusCode
    }
    catch {
        $code = 0

        if ($null -ne $_.Exception.Response) {
            try {
                $code = [int]$_.Exception.Response.StatusCode
            }
            catch {}
        }

        if ($code -eq 0) {
            Write-Host "ERREUR technique : $($_.Exception.Message)" -ForegroundColor Red
        }
    }

    Write-Host "HTTP obtenu : $code | Attendu : $($test.Attendu)"

    if ($code -eq $test.Attendu) {
        Write-Host "RESULTAT : OK" -ForegroundColor Green
    }
    else {
        Write-Host "RESULTAT : ECHEC" -ForegroundColor Red
        $echecs++
    }
}

Write-Host "`n=== BILAN ===" -ForegroundColor Cyan

if ($echecs -eq 0) {
    Write-Host "SUCCES : les trois codes HTTP correspondent aux attentes." -ForegroundColor Green
}
else {
    Write-Host "$echecs test(s) en échec." -ForegroundColor Red
}

$adminToken = $null
$secureToken = $null