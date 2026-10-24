-- Stage 4 durable inbox for at-least-once JetStream delivery.
BEGIN;
CREATE TABLE IF NOT EXISTS processed_events (
    event_id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMIT;
