# ADR-0001: Asynchronous messaging on SQS standard + DLQ (reject Kafka/MSK)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0002 (Compute), ADR-0003 (Ledger consistency model)

## Context

Settlement is asynchronous: after the `api` reserves funds synchronously, it enqueues a
settlement job that the `worker` consumes to finalize the transaction and dispatch a
signed webhook. We need a durable queue between them with:

- at-least-once delivery and a dead-letter path for poison messages,
- a backlog signal we can autoscale the worker on,
- low fixed cost and near-zero operational weight (this is a cost-conscious, ephemeral,
  spin-up/tear-down portfolio project).

Candidates: **SQS standard**, **SQS FIFO**, **Amazon MSK / self-managed Kafka**.

## Decision

Use an **SQS standard** queue with a redrive policy to a dedicated **DLQ**.

- At-least-once delivery is embraced, not fought: the consumer is made **idempotent**
  keyed on `transfer_id` (`UNIQUE` settlement entry — see ADR-0003), so redelivery cannot
  double-spend.
- Retry/backoff for a message is delegated to SQS visibility timeout + redrive; poison
  messages move to the DLQ after `maxReceiveCount`.
- Worker autoscaling is driven by SQS backlog metrics (`ApproximateNumberOfMessagesVisible`
  / `ApproximateAgeOfOldestMessage`), not CPU.

## Rationale

- **SQS standard vs FIFO:** FIFO offers ordering and scoped exactly-once *processing*, but
  it is throughput-limited and, more importantly, leaning on FIFO dedup would *hide* the
  idempotency/dedup engineering the project is meant to demonstrate. Money-movement
  correctness should come from the ledger's constraints, not from a queue feature. Standard
  is also cheaper and higher-throughput. Ordering is not required: each transfer is
  independent and keyed by `transfer_id`.
- **SQS vs Kafka/MSK:** Kafka's strengths — an ordered, long-retention, replayable log with
  high fan-out to many independent consumer groups — are real, but this workload has one
  producer, one logical consumer, and no replay/stream-processing requirement. MSK adds
  standing broker cost and operational surface (brokers, partitions, ISR, rebalancing) that
  contradicts the ephemeral, cost-conscious design. A managed queue is the right-sized tool.

## Consequences

Positive:
- Minimal fixed cost; no brokers to operate; pay-per-request fits the ephemeral lifecycle.
- Native DLQ + redrive and first-class CloudWatch backlog metrics for autoscaling.
- Forces (and showcases) idempotent-consumer design.

Negative / risks:
- No total ordering and no built-in message replay/retention beyond the queue.
- At-least-once pushes correctness onto the application — mitigated by the `transfer_id`
  unique constraint and idempotent finalize.

## Production equivalent / when this flips

If the platform grew to need an ordered, replayable event log consumed by many independent
services (fraud scoring, analytics, reconciliation, notifications) or event sourcing, Kafka
(MSK) or EventBridge Pipes + a log store becomes justified. For a single-producer /
single-consumer settlement path, SQS is the correct, defensible choice.

## Alternatives considered

- **SQS FIFO:** throughput-limited; would mask the idempotency work; ordering not needed.
- **Amazon MSK / Kafka:** standing cost and operational weight unjustified for one
  consumer with no replay requirement.
- **EventBridge:** great for routing/fan-out of events, but not the durable work-queue +
  DLQ + backlog-autoscaling primitive we need here.
