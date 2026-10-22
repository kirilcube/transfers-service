# transfers-service

This project is a distributed backend service designed to process SEPA/UK transfers. The core focus is on exactly-once semantics, data integrity, and system resilience under failure.

It models my estimation of how fintechs like SumUp process real-world transfers:

1. Transfer verification
2. Reserving funds in the ledger
3. Routing to a partner bank's adapter
4. Receiving an asynchronous response
5. Settling funds in the ledger
6. Notifying the user

*(Note: The Partner Bank represents a real banking entity with access to clearing networks. Its adapter—which translates our JSON to ISO 20022 `.pain`/`.pacs` files and produces messages to a `cleared` topic—is not implemented in current iteration).*

## Pipeline

The system implements the SEPA Credit Transfer (SCT) flow. The happy path executes in two asynchronous phases:

### First Phase: Transfer Initiation

1. Accept HTTP request from the Gateway API.
2. Insert a new transfer into the database.
3. Perform Verification of Payee (VoP).
4. Perform Antifraud verification.
5. Route to the appropriate partner bank.
6. Update status to `INITIALIZED`.
7. Reserve funds in the Ledger.
8. Update status and write an outbox event for the partner bank's adapter.

### Second Phase: Settlement

9. Consume the cleared message from the `sct.cleared` Kafka topic.
10. Settle funds in the Ledger.
11. Update status to `SETTLED` and write an outbox event to the `notifications` topic.
12. Commit the Kafka offset.

### Resilience Mechanisms

Two core mechanisms ensure every transfer reaches a terminal state (either failed or settled), even if a pod crashes mid-execution:

* **The Sweeper (First Phase):** Recovers transfers left in an unfinished state. It uses a `locked_at` timestamp column to find abandoned transfers, updates a `version` column to claim them (optimistic locking), and re-queues them into the pipeline.
* **Manual Offset Commits (Second Phase):** A transfer's `settled` message is not committed to Kafka until the database transaction fully succeeds. If the pod crashes, Kafka will re-deliver the message to another pod.

## Idempotency

The system enforces strict deduplication across three layers:

* **API Layer:** We use an `idempotency_records` table and the `Idempotency-Key` HTTP header to deduplicate incoming transfer requests.
* **Settlement Layer:** We use `transfer_id` + the current database state to deduplicate incoming Kafka settlement messages.
* **Ledger Layer:** We generate deterministically derived idempotency keys for all outgoing requests to the Ledger.

## Scope

External domain integrations—Verification of Payee (VoP), Antifraud, Routing, the Ledger, and Partner Bank adapters—are mocked. I treat these as external microservices connected via gRPC or Kafka.

### The Ledger Mock

The Ledger is mocked using a simple, single-table database implementation. This provides exactly what is needed to support advanced chaos testing (including pod kills) with minimum complexity.

*(A production-grade ledger should be implemented using double-entry accounting and transit accounts. See [Stripe's Ledger architecture](https://stripe.dev/blog/ledger-stripe-system-for-tracking-and-validating-money-movement) for reference).*

## Infrastructure

* **PostgreSQL:** Primary datastore.
* **Kafka:** Event streaming and asynchronous messaging.
* **Debezium:** Change Data Capture (CDC) for the Transactional Outbox pattern.

### Considered / Future Improvements

*   **Clearing Rejections:** If a transfer is rejected in clearing, the system needs a rollback flow to execute compensating transactions.
*   **E2E & Chaos Testing Are Fragile** Currently, the E2E and Chaos Testing developer experience requires heavy manual orchestration. It should be addressed in the future.
*   **VoP Override Flow:** Currently, if Verification of Payee (VoP) returns a `no_match`, the system halts the transfer and returns a "VoP override required" error. The follow-up API endpoint for clients to explicitly confirm the override and resume the transfer is not yet implemented.
*   **Redis:** A must-have for production, but prioritized behind core state-machine logic for this iteration.
*   **Metrics & Observability:** While Prometheus/Grafana metrics are essential for a live system, instrumentation was omitted to keep the codebase strictly focused on proving data integrity and exactly-once semantics.
*   **Rate-Limiter:** Relevant for a deployed environment under stress testing, but out of scope for the current architectural goals.

## Quick Start

The infrastructure and test commands are managed via a `Makefile` located in the `/deploy` directory.

**Available Commands:**

* `make up` — Starts all infrastructure dependencies in the background (`docker-compose up -d`).
* `make test` — Runs the unit and integration test suite with the race detector enabled (`go test -v -race ./...`).
* `make reset` — Completely wipes the current state. Useful for clearing between manual E2E test runs.
* `make down` — Stops the infrastructure without removing volumes.
* `make clean` — Stops the infrastructure and removes all associated data volumes.

## Testing

The test suite ensures data integrity across both happy and error paths, relying heavily on real database engines rather than mocks.

### Integration Testing (Unit Level)

The service layer tests use `testcontainers-go` to spin up a real PostgreSQL database for every test. We verify:

* Successfully accepting a new transfer.
* Failing and rejecting duplicate transfers (Idempotency conflicts).
* Failing with domain error when Antifraud fails.
* Failing with domain errors when VoP No-Match.
* Successfully settling a transfer.
* Idempotently handling duplicate settlement attempts.

*Verification:* At the end of each test, we assert the Ledger's exact state to ensure no ghost funds were reserved or settled.

### End-to-End & Chaos Testing

E2E scripts simulate extreme failure scenarios to validate the idempotency keys, database locks, and CDC Outbox resilience:

*   **Crash Recovery (`TestDropPodTransfers`):** A pod is violently killed (`docker kill`) while processing a massive batch of incoming transfers. The pod is restarted, and the exact same batch is re-sent to verify absolute idempotency and outbox recovery.
*   **Concurrency (`TestTwoPods`):** Two pods simultaneously receive the exact same HTTP New Transfer requests to test race conditions and unique constraints.

**Current Test Orchestration (Manual)**
Currently these tests require manual orchestration: 
*   **Execution:** Tests must be run strictly one by one using a regex target:
    `go test -v -tags=e2e ./e2e -run ^TestDropPodTransfers$`
*   **State Wipes:** The entire Docker environment must be manually reset before every test run.
*   **Topology:** To run the concurrency test, you must manually uncomment the second pod in `deploy/docker-compose.yml`.
*   **Scope:** The E2E assertions currently only support zero-fee transfers.