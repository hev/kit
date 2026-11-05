@echo off
rem Test-only source. events leaves a grandchild that outlives cmd.exe.
if "%~1"=="events" goto events
if "%~1"=="--hev-describe" echo {"source":true}
exit /b 0
:events
echo diagnostic line 1>&2
powershell -NoProfile -NonInteractive -Command "Set-Content -Path (Join-Path $env:HEV_TEST_DIR started) -Value x; Start-Sleep -Seconds 6; Set-Content -Path (Join-Path $env:HEV_TEST_DIR survived) -Value x"
