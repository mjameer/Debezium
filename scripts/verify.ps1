$ErrorActionPreference = "Continue"

Write-Host "=== postgres1 (source) ===" -ForegroundColor Yellow
podman exec postgres1 psql -U postgres -d omedb -c "SELECT 'devices' t,count(*) FROM devices UNION ALL SELECT 'alerts',count(*) FROM alerts UNION ALL SELECT 'device_health',count(*) FROM device_health UNION ALL SELECT 'groups',count(*) FROM groups ORDER BY t;"

Write-Host "`n=== postgres2 (target) ===" -ForegroundColor Yellow
podman exec postgres2 psql -U postgres -d omedb -c "SELECT 'devices' t,count(*) FROM devices UNION ALL SELECT 'alerts',count(*) FROM alerts UNION ALL SELECT 'device_health',count(*) FROM device_health UNION ALL SELECT 'groups',count(*) FROM groups ORDER BY t;"

Write-Host "`n=== Live Test ===" -ForegroundColor Yellow
$ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
podman exec postgres1 psql -U postgres -d omedb -c "INSERT INTO devices(service_tag,device_name,device_type_id,model,ip_address,health_status) VALUES('LIVE${ts}','live.local',1,'PowerEdge R750','10.88.1.1','OK');"
Write-Host "Inserted LIVE${ts}. Waiting 5s..."
Start-Sleep -Seconds 5
podman exec postgres2 psql -U postgres -d omedb -c "SELECT id,service_tag,model,health_status FROM devices WHERE service_tag='LIVE${ts}';"
