DROP TRIGGER IF EXISTS ledger_entries_immutable ON ledger_entries;
DROP TRIGGER IF EXISTS ledger_transactions_immutable ON ledger_transactions;
DROP FUNCTION IF EXISTS reject_ledger_mutation();
DROP TABLE IF EXISTS payment_webhook_receipts;
DROP INDEX IF EXISTS ux_ledger_transaction_reference;
DROP INDEX IF EXISTS ux_invoices_order;
DROP INDEX IF EXISTS ux_orders_user_idempotency;
ALTER TABLE orders DROP COLUMN IF EXISTS idempotency_key;
