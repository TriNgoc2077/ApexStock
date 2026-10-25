# Disaster Recovery Checklist

1. **Verify Container Status:**
   `docker compose ps`
2. **Inspect Container Logs for Panics:**
   `docker compose logs --tail=50 <service_name>`
3. **Database Reconnection Check:**
   `docker exec -it apexstock-postgres-1 psql -U apexstock -c "SELECT count(*) FROM pg_stat_activity;"`
4. **Redis Cache Rebuild & Warmup:**
   `./scripts/warmup-redis.sh`
5. **Execute Invariant Verification:**
   `./scripts/reconcile.sql.sh`
