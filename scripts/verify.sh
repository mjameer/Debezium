#!/usr/bin/env bash
set -euo pipefail
echo "=== postgres1 (source) ==="
podman exec postgres1 psql -U postgres -d omedb -c "SELECT 'devices' t,count(*) FROM devices UNION ALL SELECT 'alerts',count(*) FROM alerts UNION ALL SELECT 'device_health',count(*) FROM device_health UNION ALL SELECT 'groups',count(*) FROM groups ORDER BY t;"
echo ""
echo "=== postgres2 (target) ==="
podman exec postgres2 psql -U postgres -d omedb -c "SELECT 'devices' t,count(*) FROM devices UNION ALL SELECT 'alerts',count(*) FROM alerts UNION ALL SELECT 'device_health',count(*) FROM device_health UNION ALL SELECT 'groups',count(*) FROM groups ORDER BY t;"
echo ""
echo "=== Live Test ==="
TS=$(date +%s)
podman exec postgres1 psql -U postgres -d omedb -c "INSERT INTO devices(service_tag,device_name,device_type_id,model,ip_address,health_status) VALUES('LIVE${TS}','live.local',1,'PowerEdge R750','10.88.1.1','OK');"
echo "Inserted LIVE${TS}. Waiting 5s..."
sleep 5
podman exec postgres2 psql -U postgres -d omedb -c "SELECT id,service_tag,model,health_status FROM devices WHERE service_tag='LIVE${TS}';"
