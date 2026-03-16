$ErrorActionPreference = "Stop"
$base = "http://localhost:3000"
$secret = "Bearer change-this-secret-key-in-production"
$pass = 0; $fail = 0

function Test($name, $expr, $expect) {
    $result = & $expr
    if ($result -eq $expect) {
        Write-Host "  PASS  $name" -ForegroundColor Green
        $script:pass++
    } else {
        Write-Host "  FAIL  $name (got: $result, expected: $expect)" -ForegroundColor Red
        $script:fail++
    }
}

function Req($method, $uri, $token, $body) {
    $h = @{}
    if ($token) { $h["Authorization"] = $token }
    $params = @{ Uri = "$base$uri"; Method = $method; UseBasicParsing = $true; Headers = $h }
    if ($body) { $params["Body"] = $body; $params["ContentType"] = "application/json" }
    try {
        $r = Invoke-WebRequest @params
        return @{ Status = $r.StatusCode; Body = ($r.Content | ConvertFrom-Json) }
    } catch {
        $code = [int]$_.Exception.Response.StatusCode
        try {
            $stream = $_.Exception.Response.GetResponseStream()
            $reader = New-Object System.IO.StreamReader($stream)
            $bodyText = $reader.ReadToEnd()
            $bodyObj = $bodyText | ConvertFrom-Json
        } catch { $bodyObj = $null }
        return @{ Status = $code; Body = $bodyObj }
    }
}

Write-Host "`n========================================" -ForegroundColor Cyan
Write-Host "  RBAC Validation Suite" -ForegroundColor Cyan
Write-Host "========================================`n" -ForegroundColor Cyan

# ── 1. Health (no auth) ──────────────────────────
Write-Host "[Health]" -ForegroundColor Yellow
$r = Req "GET" "/api/health" $null $null
Test "Health returns 200" { $r.Status } 200
Test "Health body ok" { $r.Body.status } "ok"

# ── 2. No auth → 401 ────────────────────────────
Write-Host "`n[Auth: no token]" -ForegroundColor Yellow
$r = Req "GET" "/api/v1/accounts" $null $null
Test "No token → 401" { $r.Status } 401

# ── 3. Bad token → 401 ──────────────────────────
Write-Host "`n[Auth: bad token]" -ForegroundColor Yellow
$r = Req "GET" "/api/v1/accounts" "Bearer wrong-token" $null
Test "Bad token → 401" { $r.Status } 401

# ── 4. Secret key (system admin) ─────────────────
Write-Host "`n[Auth: secret key (admin)]" -ForegroundColor Yellow
$r = Req "GET" "/api/v1/accounts" $secret $null
Test "Secret key → 200" { $r.Status } 200
Test "Admin sees accounts" { $r.Body.total -ge 0 } $true

# ── 5. Roles ─────────────────────────────────────
Write-Host "`n[Roles]" -ForegroundColor Yellow
$r = Req "GET" "/api/v1/roles" $secret $null
Test "List roles → 200" { $r.Status } 200
Test "2 built-in roles" { $r.Body.total } 2
Test "Admin role exists" { ($r.Body.roles | Where-Object name -eq "admin").is_builtin } $true
Test "User role exists" { ($r.Body.roles | Where-Object name -eq "user").is_builtin } $true
Test "Admin has * perm" { ($r.Body.roles | Where-Object name -eq "admin").permissions -contains "*" } $true

# ── 6. Create custom role ────────────────────────
Write-Host "`n[Create custom role]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/roles" $secret '{"name":"viewer","description":"Read-only access","permissions":["accounts:read","chats:read","messages:read","contacts:read","groups:read","profile:read"]}'
Test "Create role → 201" { $r.Status } 201
Test "Role name = viewer" { $r.Body.name } "viewer"
$viewerRoleId = $r.Body.id

# ── 7. Login (no users) ─────────────────────────
Write-Host "`n[Login: no matching user]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/auth/login" $null '{"username":"nobody","password":"nopassword1"}'
Test "Login unknown user → 401" { $r.Status } 401

# ── 8. Delete old test users if any ──────────────
$r = Req "GET" "/api/v1/users" $secret $null
foreach ($u in $r.Body.users) {
    if ($u.username -eq "alice" -or $u.username -eq "bob") {
        Req "DELETE" "/api/v1/users/$($u.id)" $secret $null | Out-Null
    }
}

# ── 9. Create users ─────────────────────────────
Write-Host "`n[Create users]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/users" $secret '{"username":"alice","password":"alicepass123","role_id":"builtin-admin"}'
Test "Create alice (admin) → 201" { $r.Status } 201
$aliceId = $r.Body.id
Test "Alice role = admin" { $r.Body.role_name } "admin"

$r = Req "POST" "/api/v1/users" $secret '{"username":"bob","password":"bobsecure123","role_id":"builtin-user"}'
Test "Create bob (user) → 201" { $r.Status } 201
$bobId = $r.Body.id
Test "Bob role = user" { $r.Body.role_name } "user"

# ── 10. Duplicate username ───────────────────────
Write-Host "`n[Duplicate username]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/users" $secret '{"username":"alice","password":"whatever123","role_id":"builtin-user"}'
Test "Duplicate username → 409" { $r.Status } 409

# ── 11. Validation ───────────────────────────────
Write-Host "`n[Validation]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/users" $secret '{"username":"x","password":"short","role_id":"builtin-user"}'
Test "Short password → 400" { $r.Status } 400

$r = Req "POST" "/api/v1/users" $secret '{"username":"x","password":"longpassword","role_id":"nonexistent"}'
Test "Bad role_id → 400" { $r.Status } 400

# ── 12. Login as alice (admin user) ──────────────
Write-Host "`n[Login: alice (admin)]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/auth/login" $null '{"username":"alice","password":"alicepass123"}'
Test "Alice login → 200" { $r.Status } 200
Test "Token returned" { $r.Body.token.Length -gt 20 } $true
$aliceToken = "Bearer $($r.Body.token)"

# ── 13. Login as bob (user) ─────────────────────
Write-Host "`n[Login: bob (user)]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/auth/login" $null '{"username":"bob","password":"bobsecure123"}'
Test "Bob login → 200" { $r.Status } 200
$bobToken = "Bearer $($r.Body.token)"

# ── 14. Wrong password ──────────────────────────
Write-Host "`n[Login: wrong password]" -ForegroundColor Yellow
$r = Req "POST" "/api/v1/auth/login" $null '{"username":"alice","password":"wrongpassword"}'
Test "Wrong password → 401" { $r.Status } 401

# ── 15. Alice (admin) permissions ────────────────
Write-Host "`n[Alice: admin permissions]" -ForegroundColor Yellow
$r = Req "GET" "/api/v1/accounts" $aliceToken $null
Test "Alice lists accounts → 200" { $r.Status } 200

$r = Req "GET" "/api/v1/users" $aliceToken $null
Test "Alice lists users → 200" { $r.Status } 200

$r = Req "GET" "/api/v1/roles" $aliceToken $null
Test "Alice lists roles → 200" { $r.Status } 200

# ── 16. Bob (user) permissions ───────────────────
Write-Host "`n[Bob: user permissions]" -ForegroundColor Yellow
$r = Req "GET" "/api/v1/accounts" $bobToken $null
Test "Bob lists accounts → 200" { $r.Status } 200
Test "Bob sees 0 accounts (none assigned)" { $r.Body.total } 0

$r = Req "GET" "/api/v1/users" $bobToken $null
Test "Bob lists users → 403" { $r.Status } 403

$r = Req "GET" "/api/v1/roles" $bobToken $null
Test "Bob lists roles → 403" { $r.Status } 403

$r = Req "POST" "/api/v1/users" $bobToken '{"username":"hacker","password":"hacking123","role_id":"builtin-admin"}'
Test "Bob creates user → 403" { $r.Status } 403

$r = Req "POST" "/api/v1/accounts" $bobToken '{"phone_number":"+911111111111","account_name":"hack"}'
Test "Bob creates account → 403" { $r.Status } 403

# ── 17. Bob can read own profile ─────────────────
Write-Host "`n[Bob: self-access]" -ForegroundColor Yellow
$r = Req "GET" "/api/v1/users/$bobId" $bobToken $null
Test "Bob reads own user → 200" { $r.Status } 200
Test "Bob sees own username" { $r.Body.username } "bob"

$r = Req "GET" "/api/v1/users/$aliceId" $bobToken $null
Test "Bob reads alice → 403" { $r.Status } 403

# ── 18. Update user ─────────────────────────────
Write-Host "`n[Update user]" -ForegroundColor Yellow
$r = Req "PATCH" "/api/v1/users/$bobId" $secret '{"enabled":false}'
Test "Disable bob → 200" { $r.Status } 200
Test "Bob disabled" { $r.Body.enabled } $false

# Disabled user can't login
$r = Req "POST" "/api/v1/auth/login" $null '{"username":"bob","password":"bobsecure123"}'
Test "Disabled bob login → 403" { $r.Status } 403

# Re-enable
$r = Req "PATCH" "/api/v1/users/$bobId" $secret '{"enabled":true}'
Test "Re-enable bob → 200" { $r.Status } 200

# ── 19. Delete built-in role → blocked ───────────
Write-Host "`n[Protect built-in roles]" -ForegroundColor Yellow
$r = Req "DELETE" "/api/v1/roles/builtin-admin" $secret $null
Test "Delete admin role → 403" { $r.Status } 403

$r = Req "DELETE" "/api/v1/roles/builtin-user" $secret $null
Test "Delete user role → 403" { $r.Status } 403

# ── 20. Delete custom role (has no users) ────────
Write-Host "`n[Delete custom role]" -ForegroundColor Yellow
if ($viewerRoleId) {
    $r = Req "DELETE" "/api/v1/roles/$viewerRoleId" $secret $null
    Test "Delete viewer role → 200" { $r.Status } 200
}

# ── 21. Cleanup test users ──────────────────────
Write-Host "`n[Cleanup]" -ForegroundColor Yellow
$r = Req "DELETE" "/api/v1/users/$aliceId" $secret $null
Test "Delete alice → 200" { $r.Status } 200

$r = Req "DELETE" "/api/v1/users/$bobId" $secret $null
Test "Delete bob → 200" { $r.Status } 200

# ── Summary ──────────────────────────────────────
Write-Host "`n========================================" -ForegroundColor Cyan
Write-Host "  Results: $pass passed, $fail failed" -ForegroundColor $(if ($fail -eq 0) { "Green" } else { "Red" })
Write-Host "========================================`n" -ForegroundColor Cyan

exit $fail
