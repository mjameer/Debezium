# 1-reader — Source OME Database

**What**: PostgreSQL with 13 OME tables and ~8,500 rows of seed data.

**Role**: This is the **READ source**. Debezium (in `2-kafka-debezium/`) reads this database's WAL.

**Does NOT**: Write anywhere. It just holds data and produces WAL entries when you insert/update/delete.

## Run

```bash
cd 1-reader
podman compose up -d
```

## Verify data is there

```bash
podman exec postgres1 psql -U postgres -d omedb -c \
  "SELECT 'devices' t, count(*) FROM devices UNION ALL
   SELECT 'alerts', count(*) FROM alerts UNION ALL
   SELECT 'device_health', count(*) FROM device_health
   ORDER BY t;"
```

## Tables

| Table | Rows | Description |
|---|---|---|
| device_types | 10 | PowerEdge, PowerSwitch, PowerStore, etc. |
| devices | 500 | Servers, switches, storage |
| device_inventory | ~3,000 | CPU, Memory, NIC, PSU per device |
| groups | 10 | Device group hierarchy |
| group_memberships | ~700 | Device-to-group links |
| alert_categories | 5 | System Health, Storage, Config, Audit |
| alerts | 2,000 | Critical/Warning/Info alerts |
| device_health | 500 | Per-device health metrics |
| firmware_catalog | 18 | Firmware entries |
| compliance_baselines | 3 | Config compliance templates |
| compliance_results | 600 | Per-device compliance |
| users | 5 | OME admin users |
| job_history | 100 | Job execution records |
