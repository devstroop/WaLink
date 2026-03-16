$ErrorActionPreference = "Stop"
$base = "http://localhost:3000"
$secret = "change-this-secret-key-in-production"
$adminH = @{Authorization = "Bearer $secret"; "Content-Type" = "application/json"}
$pass = 0; $fail = 0; $total = 0

function Test($name, $block) {
    $script:total++
    try {
        & $block
        $script:pass++
        Write-Host "  PASS  $name" -ForegroundColor Green
    } catch {
        $script:fail++
        Write-Host "  FAIL  $name -- $($_.Exception.Message)" -ForegroundColor Red
    }
}

function Assert-Equal($a, $b, $msg) {
    if ($a -ne $b) { throw "Expected '$b' got '$a' -- $msg" }
}
function Assert-NotNull($a, $msg) {
    if ($null -eq $a -or $a -eq "") { throw "Expected non-null -- $msg" }
}
function Assert-StatusCode($uri, $method, $headers, $body, $expected) {
    try {
        $params = @{Uri=$uri; Method=$method; Headers=$headers}
        if ($body) { $params.Body = $body }
        Invoke-RestMethod @params | Out-Null
        if ($expected -ge 400) { throw "Expected $expected but got 2xx" }
    } catch {
        $code = $_.Exception.Response.StatusCode.value__
        if ($code -ne $expected) { throw "Expected $expected got $code" }
    }
}

Write-Host ""
Write-Host "=== RBAC + API Keys Integration Tests ===" -ForegroundColor Cyan
Write-Host "Base: $base"
Write-Host ""

# --- Setup ---
Write-Host "--- Setup ---" -ForegroundColor Yellow

$adminUser = Invoke-RestMethod -Uri "$base/api/v1/users" -Method POST -Headers $adminH -Body '{"username":"testadmin","password":"Admin1234!","role_id":"builtin-admin"}'
$stdUser = Invoke-RestMethod -Uri "$base/api/v1/users" -Method POST -Headers $adminH -Body '{"username":"testuser","password":"User1234!","role_id":"builtin-user"}'
$stdUser2 = Invoke-RestMethod -Uri "$base/api/v1/users" -Method POST -Headers $adminH -Body '{"username":"testuser2","password":"User1234!","role_id":"builtin-user"}'

$acct1 = Invoke-RestMethod -Uri "$base/api/v1/accounts" -Method POST -Headers $adminH -Body "{`"account_name`":`"Account1`",`"phone_number`":`"919000000001`",`"user_id`":`"$($stdUser.id)`"}"
$acct2 = Invoke-RestMethod -Uri "$base/api/v1/accounts" -Method POST -Headers $adminH -Body "{`"account_name`":`"Account2`",`"phone_number`":`"919000000002`",`"user_id`":`"$($stdUser.id)`"}"
$acct3 = Invoke-RestMethod -Uri "$base/api/v1/accounts" -Method POST -Headers $adminH -Body "{`"account_name`":`"Account3`",`"phone_number`":`"919000000003`",`"user_id`":`"$($stdUser2.id)`"}"

$loginStd = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method POST -Headers @{"Content-Type"="application/json"} -Body '{"username":"testuser","password":"User1234!"}'
$loginStd2 = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method POST -Headers @{"Content-Type"="application/json"} -Body '{"username":"testuser2","password":"User1234!"}'
$loginAdmin = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method POST -Headers @{"Content-Type"="application/json"} -Body '{"username":"testadmin","password":"Admin1234!"}'

$jwtStd = $loginStd.token
$jwtStd2 = $loginStd2.token
$jwtAdmin = $loginAdmin.token

$stdH = @{Authorization = "Bearer $jwtStd"; "Content-Type" = "application/json"}
$std2H = @{Authorization = "Bearer $jwtStd2"; "Content-Type" = "application/json"}
$admH = @{Authorization = "Bearer $jwtAdmin"; "Content-Type" = "application/json"}

Write-Host "  Users: admin=$($adminUser.id), user1=$($stdUser.id), user2=$($stdUser2.id)"
Write-Host "  Accounts: a1=$($acct1.id), a2=$($acct2.id), a3=$($acct3.id)"

# --- Auth Tests ---
Write-Host ""
Write-Host "--- Auth ---" -ForegroundColor Yellow

Test "Secret key auth works" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/users" -Method GET -Headers $adminH
    Assert-NotNull $r.total "user list"
}

Test "JWT auth works" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method GET -Headers $stdH
    Assert-Equal $r.total 0 "no keys yet"
}

Test "Missing auth header returns 401" {
    Assert-StatusCode "$base/api/v1/api-keys" "GET" @{} $null 401
}

Test "Invalid token returns 401" {
    Assert-StatusCode "$base/api/v1/api-keys" "GET" @{Authorization="Bearer invalid"} $null 401
}

# --- API Key CRUD ---
Write-Host ""
Write-Host "--- API Key CRUD ---" -ForegroundColor Yellow

Test "Create basic API key (no account binding)" {
    $script:key1 = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method POST -Headers $stdH -Body '{"name":"Basic Key"}'
    Assert-NotNull $key1.key "key returned"
    Assert-Equal ($key1.key.StartsWith("walink_")) $true "walink_ prefix"
    Assert-Equal $key1.name "Basic Key" "name"
}

Test "Create account-bound API key" {
    $body = '{"name":"Bound Key","account_id":"' + $acct1.id + '"}'
    $script:key2 = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method POST -Headers $stdH -Body $body
    Assert-NotNull $key2.key "key returned"
    Assert-Equal $key2.account_id $acct1.id "bound to account"
}

Test "Create key with expiry" {
    $exp = (Get-Date).AddDays(30).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $body = '{"name":"Expiring Key","expires_at":"' + $exp + '"}'
    $script:key3 = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method POST -Headers $stdH -Body $body
    Assert-NotNull $key3.expires_at "has expiry"
}

Test "Cannot create key without name" {
    Assert-StatusCode "$base/api/v1/api-keys" "POST" $stdH '{"name":""}' 400
}

Test "Cannot bind key to account user doesnt own" {
    $body = '{"name":"Bad Bind","account_id":"' + $acct3.id + '"}'
    Assert-StatusCode "$base/api/v1/api-keys" "POST" $stdH $body 403
}

Test "Cannot bind key to non-existent account" {
    Assert-StatusCode "$base/api/v1/api-keys" "POST" $stdH '{"name":"Bad","account_id":"nonexistent"}' 404
}

Test "List keys shows all users keys" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method GET -Headers $stdH
    Assert-Equal $r.total 3 "3 keys created"
}

Test "Key list includes account_id" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method GET -Headers $stdH
    $bound = $r.keys | Where-Object { $_.name -eq "Bound Key" }
    Assert-Equal $bound.account_id $acct1.id "account_id in list"
}

Test "User2 sees no keys (only own)" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method GET -Headers $std2H
    Assert-Equal $r.total 0 "user2 has 0 keys"
}

# --- API Key Auth ---
Write-Host ""
Write-Host "--- API Key Auth ---" -ForegroundColor Yellow

Test "Basic API key authenticates to REST API" {
    $keyH = @{Authorization = "Bearer $($key1.key)"}
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method GET -Headers $keyH
    Assert-Equal $r.total 3 "sees own keys"
}

Test "Account-bound API key authenticates to REST API" {
    $keyH = @{Authorization = "Bearer $($key2.key)"}
    $r = Invoke-RestMethod -Uri "$base/api/v1/accounts" -Method GET -Headers $keyH
    Assert-Equal $r.total 2 "sees own accounts"
}

Test "API key inherits user permissions (user cannot list users)" {
    $keyH = @{Authorization = "Bearer $($key1.key)"}
    Assert-StatusCode "$base/api/v1/users" "GET" $keyH $null 403
}

Test "Revoked API key is rejected" {
    $tmpKey = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method POST -Headers $stdH -Body '{"name":"tmp"}'
    Invoke-RestMethod -Uri "$base/api/v1/api-keys/$($tmpKey.id)" -Method DELETE -Headers $stdH | Out-Null
    Assert-StatusCode "$base/api/v1/api-keys" "GET" @{Authorization="Bearer $($tmpKey.key)"} $null 401
}

Test "Invalid API key format rejected" {
    Assert-StatusCode "$base/api/v1/api-keys" "GET" @{Authorization="Bearer walink_invalid"} $null 401
}

# --- API Key Ownership ---
Write-Host ""
Write-Host "--- API Key Ownership ---" -ForegroundColor Yellow

Test "User can delete own key" {
    $tmp = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method POST -Headers $stdH -Body '{"name":"deleteme"}'
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys/$($tmp.id)" -Method DELETE -Headers $stdH
    Assert-Equal $r.status "deleted" "deleted"
}

Test "User cannot delete other users key" {
    Assert-StatusCode "$base/api/v1/api-keys/$($key1.id)" "DELETE" $std2H $null 403
}

Test "Admin can delete any users key" {
    $tmp = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method POST -Headers $stdH -Body '{"name":"admin-del"}'
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys/$($tmp.id)" -Method DELETE -Headers $admH
    Assert-Equal $r.status "deleted" "admin deleted user key"
}

Test "Delete non-existent key returns 404" {
    Assert-StatusCode "$base/api/v1/api-keys/nonexistent" "DELETE" $stdH $null 404
}

# --- MCP Scoping ---
Write-Host ""
Write-Host "--- MCP Scoping ---" -ForegroundColor Yellow

$mcpInit = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}'
$mcpCT = @{"Content-Type"="application/json"; Accept="application/json"}

Test "Admin: MCP works without account_id" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $secret"
    $r = Invoke-RestMethod -Uri "$base/mcp" -Method POST -Headers $h -Body $mcpInit
    Assert-Equal $r.result.serverInfo.name "WaLink" "MCP init"
}

Test "Admin: MCP works with account_id param" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $secret"
    $r = Invoke-RestMethod -Uri "$base/mcp?account_id=$($acct1.id)" -Method POST -Headers $h -Body $mcpInit
    Assert-Equal $r.result.serverInfo.name "WaLink" "MCP init scoped"
}

Test "User JWT: MCP fails without account_id" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $jwtStd"
    Assert-StatusCode "$base/mcp" "POST" $h $mcpInit 400
}

Test "User JWT: MCP works with own account_id" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $jwtStd"
    $r = Invoke-RestMethod -Uri "$base/mcp?account_id=$($acct1.id)" -Method POST -Headers $h -Body $mcpInit
    Assert-Equal $r.result.serverInfo.name "WaLink" "user scoped"
}

Test "User JWT: MCP rejects other users account_id" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $jwtStd"
    Assert-StatusCode "$base/mcp?account_id=$($acct3.id)" "POST" $h $mcpInit 403
}

Test "Account-bound key: MCP works WITHOUT account_id param" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $($key2.key)"
    $r = Invoke-RestMethod -Uri "$base/mcp" -Method POST -Headers $h -Body $mcpInit
    Assert-Equal $r.result.serverInfo.name "WaLink" "auto-scoped"
}

Test "Unbound key: MCP fails without account_id" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $($key1.key)"
    Assert-StatusCode "$base/mcp" "POST" $h $mcpInit 400
}

Test "Unbound key: MCP works with account_id param" {
    $h = $mcpCT.Clone(); $h.Authorization = "Bearer $($key1.key)"
    $r = Invoke-RestMethod -Uri "$base/mcp?account_id=$($acct1.id)" -Method POST -Headers $h -Body $mcpInit
    Assert-Equal $r.result.serverInfo.name "WaLink" "with param"
}

# --- RBAC Permissions ---
Write-Host ""
Write-Host "--- RBAC Permissions ---" -ForegroundColor Yellow

Test "Standard user can read accounts" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/accounts" -Method GET -Headers $stdH
    Assert-Equal $r.total 2 "sees own 2 accounts"
}

Test "Standard user cannot manage users" {
    Assert-StatusCode "$base/api/v1/users" "GET" $stdH $null 403
}

Test "Standard user cannot create roles" {
    Assert-StatusCode "$base/api/v1/roles" "POST" $stdH '{"name":"hack","permissions":["*"]}' 403
}

Test "Standard user cannot read roles" {
    Assert-StatusCode "$base/api/v1/roles" "GET" $stdH $null 403
}

Test "Admin user sees all accounts" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/accounts" -Method GET -Headers $admH
    Assert-Equal $r.total 3 "sees all 3"
}

Test "Admin user can manage users" {
    $r = Invoke-RestMethod -Uri "$base/api/v1/users" -Method GET -Headers $admH
    Assert-NotNull $r.total "can list users"
}

# --- Last Used Tracking ---
Write-Host ""
Write-Host "--- Last Used ---" -ForegroundColor Yellow

Test "API key last_used updates after use" {
    Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method GET -Headers @{Authorization="Bearer $($key1.key)"} | Out-Null
    Start-Sleep -Milliseconds 500
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method GET -Headers $stdH
    $k = $r.keys | Where-Object { $_.id -eq $key1.id }
    Assert-NotNull $k.last_used "last_used is set"
}

# --- Admin Cross-User ---
Write-Host ""
Write-Host "--- Admin Cross-User ---" -ForegroundColor Yellow

Test "Admin can bind key to any account" {
    $body = '{"name":"Admin Bound","account_id":"' + $acct3.id + '"}'
    $r = Invoke-RestMethod -Uri "$base/api/v1/api-keys" -Method POST -Headers $admH -Body $body
    Assert-Equal $r.account_id $acct3.id "admin bound to user2 account"
}

# --- Summary ---
Write-Host ""
Write-Host "=== Results: $pass/$total passed ===" -ForegroundColor $(if ($fail -eq 0) {"Green"} else {"Red"})
if ($fail -gt 0) {
    Write-Host "  $fail FAILED" -ForegroundColor Red
    exit 1
}
