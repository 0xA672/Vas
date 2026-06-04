@echo off
REM build.bat -- Build multitool from VAS source
REM Depends on: vas.exe, nasm, ld (MSYS2)

echo === 1. VAS: multitool.vas → multitool.asm ===
vas -target win64 multitool.vas -o multitool.asm 2>&1
if %ERRORLEVEL% neq 0 exit /b %ERRORLEVEL%

echo === 2. NASM: multitool.asm → multitool.obj ===
nasm -f win64 multitool.asm -o multitool.obj 2>&1
if %ERRORLEVEL% neq 0 exit /b %ERRORLEVEL%

echo === 3. Link: multitool.obj → multitool.exe ===
C:\msys64\usr\bin\ld.exe multitool.obj -o multitool.exe 2>&1
if %ERRORLEVEL% neq 0 exit /b %ERRORLEVEL%

echo === 4. Run ===
echo.
echo [strlen] "Hello, VAS World!"
multitool.exe
echo   strlen = %ERRORLEVEL%  (expect 17)
echo.

echo [Fibonacci] Fib(20)
echo   (switch: set MOV v0, v9 in multitool.vas)
echo.
echo [Prime] 17 is prime?
echo   (switch: set MOV v0, v5 in multitool.vas)
echo.
echo [Factorial] 8!
echo   (switch: set MOV v0, v6 in multitool.vas)
echo.
echo === Done ===
