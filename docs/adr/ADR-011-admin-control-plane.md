# ADR-011 — Administrator control plane and transactional audit

## Status

Accepted.

## Decision

The administrator surface is an independent `/api/v1/admin/*` control plane backed by the existing isolated admin session. Every handler declares one exact permission. Read models are assembled in the `admin` repository; HTTP handlers never query the database or call a Provider. Password hashes, Provider credentials/config, payment payloads, and secret setting values are excluded or masked.

High-risk user status, wallet, ticket, setting, and role changes write their Audit event in the same PostgreSQL transaction as the change. Wallet changes are immutable `admin_adjustment` Ledger transactions with equal debit and credit entries; the wallet balance remains only a projection updated in that transaction. A fixed platform adjustment account represents the counterparty.

Raw Provider operation identifiers/errors and instance diagnostics require operational management permissions in addition to the base read permission. The dashboard is anomaly-first: critical alerts, failed Operations, Provider/Node health, and capacity warnings precede commercial metrics.

## Compatibility and migration

Migration 000009 adds read/manage permissions needed by the Admin Web and assigns them to the existing V1 roles. It does not change existing permissions or domain tables. All HTTP APIs are additive.

## Verification

Integration tests verify that wallet adjustment produces balanced Ledger entries, a synchronized projection, and exactly one Audit event; secret settings remain masked. Compose acceptance logs in through the isolated admin session, exercises all read groups, performs a wallet adjustment, and checks both Ledger and Audit records. Frontend tests enforce bilingual catalog parity and the standard lint/typecheck/build Gate.
