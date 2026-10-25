# ApexStock Architectural Known Limitations

This document transparently registers all architectural constraints, scope boundaries, and known limitations of the prototype:

1. **Single-Node Redis (No Cluster/Sentinel):**
   - The current Optimized deployment utilizes a standalone Redis container. If Redis undergoes a hard unrecoverable failure, the system trips a 503 circuit breaker until manual operator intervention or container restart. Production deployments would utilize Redis Sentinel or Redis Cluster.
2. **SePay Webhook Local Tunneling:**
   - Real bank transfer webhooks from SePay require a public HTTPS URL (via Cloudflare Tunnel or ngrok). In isolated offline environments, the built-in Payment Simulator (`POST /api/v1/payments/simulate`) is used instead.
3. **In-Memory Rate Limiting Lifespan:**
   - Per-instance token buckets or rate limiters maintain counters in memory; restarting an API container resets local rate-limit windows.
4. **Single-Host CPU Parity Contention:**
   - Running full 2,000+ VU benchmarks on a single laptop host causes thermal throttling over extended durations. Benchmark runs must enforce cooling intervals (D0.11).
