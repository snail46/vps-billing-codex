# worker

Long-running process reserved for Outbox, Queue, Workflow, and Reconciler work.

Run from `backend` with `go run ./cmd/worker`. Queue processing is introduced in
the corresponding later phases; the Phase 0 process provides lifecycle and
graceful shutdown behavior only.
