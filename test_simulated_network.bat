@echo off
echo =======================================================
echo  LANShare Tek Cihazda Çift Sanal Cihaz Test Simülasyonu
echo =======================================================
echo.
echo Cihaz 1 (Alice) başlatılıyor: http://localhost:52639
start "LANShare - Alice-PC" lanshare.exe -name "Alice-PC" -id "alice-device-01" -port 52639 -tport 52638

timeout /t 2 /nobreak > nul

echo Cihaz 2 (Bob) başlatılıyor: http://localhost:52640
start "LANShare - Bob-Laptop" lanshare.exe -name "Bob-Laptop" -id "bob-device-02" -port 52640 -tport 52648

echo.
echo Başarılı! İki sanal cihaz aynı bilgisayarda başlatıldı.
echo Alice ve Bob tarayıcı sekmelerinde birbirini Radar üzerinde görecektir.
pause
