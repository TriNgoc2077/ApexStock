# Benchmark Measurement Provenance Guidelines

## Core Principles
1. **Hypothesis vs. Measured:** Any number published in documentation without an accompanying `loadtests/results/<run-id>/` artifact is classified strictly as a **HYPOTHESIS**.
2. **Provenance Metadata:** Every benchmark artifact must record:
   - Git Commit SHA
   - OS Kernel & CPU Model
   - Docker container resource limits (`docker inspect`)
   - Exact seed parameters
   - Post-run invariant verifier terminal output
3. **Sequential Execution (D0.11):** Baseline and Optimized are never measured simultaneously to ensure 100% host resource availability to the system under test.
