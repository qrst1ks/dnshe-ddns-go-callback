@echo off
setlocal enabledelayedexpansion

cd /d %~dp0

set "KEY_FILE=%CD%\.config_master_key"

if not exist "%KEY_FILE%" (
  powershell -NoProfile -Command "$b=New-Object byte[] 32; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)" > "%KEY_FILE%"
)

for /f "usebackq tokens=* delims=" %%i in ("%KEY_FILE%") do set "CONFIG_MASTER_KEY=%%i"
if "%PORT%"=="" set "PORT=18491"

echo Starting dnshe-ddns-go-callback on :%PORT%
echo Open: http://127.0.0.1:%PORT%/

if exist ".\dnshe-ddns-go-callback.exe" (
  .\dnshe-ddns-go-callback.exe
) else (
  go run .
)

endlocal
