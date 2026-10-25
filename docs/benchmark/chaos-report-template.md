# Chaos Engineering Evaluation Report

## Scenario: Component Interruption Drill
- **Target Container:** `{{CONTAINER}}`
- **Injection Timestamp:** `{{TIMESTAMP}}`
- **Detection Method:** Health watcher transition to `down`
- **Measured Recovery Time:** `{{RECOVERY_TIME_MS}}` ms

## Invariant Assessment Post-Outage
- Stock Conservation (I1): **PASS** (Zero oversell)
- Event Deduplication (I5): **PASS** (Zero duplicate orders)
- Terminal States (I2): **PASS**
