# Startup Script

Write-Host "Starting Handi Cat Wallet Tracker..." -ForegroundColor Green

# 1. Start ngrok
$ngrokProcess = Start-Process powershell -ArgumentList "ngrok http 3001" -PassThru

# Wait for ngrok
Start-Sleep -Seconds 5
$ngrokUrl = (Invoke-RestMethod http://localhost:4040/api/tunnels).tunnels[0].public_url

Write-Host "Ngrok URL: $ngrokUrl" -ForegroundColor Cyan

# 2. Update .env
$envContent = Get-Content .env -Raw
$envContent = $envContent -replace '(APP_URL=).*', "APP_URL=$ngrokUrl/webhook/telegram"
Set-Content .env -Value $envContent

# 3. Start all services
$pulseProcess = Start-Process powershell -ArgumentList "cd pulse-starter; npm start" -PassThru
$mainProcess = Start-Process powershell -ArgumentList "pnpm start" -PassThru
$studioProcess = Start-Process powershell -ArgumentList "cd pulse-starter; npx prisma studio" -PassThru

Write-Host @"
All services started:
1. Ngrok: $ngrokUrl
2. Services: Running
3. Prisma Studio: http://localhost:5555

Press Ctrl+C to stop all services
"@ -ForegroundColor Green

try {
    while ($true) { Start-Sleep -Seconds 1 }
} finally {
    $ngrokProcess, $pulseProcess, $mainProcess, $studioProcess | ForEach-Object {
        if ($_) { Stop-Process -Id $_.Id -Force }
    }
    Write-Host "`nAll services stopped" -ForegroundColor Yellow
}
