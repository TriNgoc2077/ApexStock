# ApexStock Prometheus Metric Catalog

## Overview
All metrics follow the Prometheus standard format, using bounded cardinality labels to ensure zero memory leaks during stress testing.

| Metric Name | Type | Labels | Description | Alert Threshold |
| :--- | :--- | :--- | :--- | :--- |
| `http_requests_total` | Counter | `arch`, `route`, `method`, `status` | Cumulative HTTP requests processed | Rate(5xx) > 1% |
| `http_request_duration_seconds` | Histogram | `arch`, `route`, `le` | Request duration in seconds (buckets 5ms–10s) | p99 > 200ms (Opt), > 1s (Base) |
| `orders_created_total` | Counter | `arch`, `status` | Flash sale orders created | N/A |
| `payments_processed_total` | Counter | `arch`, `outcome` | Payment outcomes (`success`, `failure`, `timeout`, `cancel`) | N/A |
| `stock_remaining` | Gauge | `arch`, `item_id` | Available inventory stock remaining | Drops to 0 when sold out |
| `nats_queue_lag` | Gauge | `stream`, `consumer` | Unconsumed messages in JetStream queue | > 1,000 messages |
| `oversell_total` | Counter | `item_id` | **CRITICAL INVARIANT:** Violations where sold stock > initial inventory | `> 0` triggers Page 1 Alert |

## Cardinality Guarantee
- `arch`: strictly `baseline` or `optimized` (2 values).
- `route`: static normalized path template (e.g. `/api/v1/flash-sale/orders`, `/healthz`, `/metrics`).
- `status`: HTTP response status code (`201`, `400`, `409`, `429`, `500`).
