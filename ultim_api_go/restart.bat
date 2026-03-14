@echo off
set PORT=9953
echo Stopping existing Go server on port %PORT%...

for /f "tokens=5" %%a in ('netstat -ano ^| findstr :%PORT% ^| findstr LISTENING') do (
    taskkill /F /PID %%a 2>nul
)

echo Starting Go backend on http://localhost:%PORT%...
go run main.go > server_log.txt 2>&1
pause
