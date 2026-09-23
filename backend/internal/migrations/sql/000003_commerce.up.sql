ALTER TABLE orders ADD COLUMN idempotency_key varchar(255);
CREATE UNIQUE INDEX ux_orders_user_idempotency ON orders(user_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX ux_invoices_order ON invoices(order_id) WHERE order_id IS NOT NULL;
CREATE UNIQUE INDEX ux_ledger_transaction_reference
ON ledger_transactions(type, reference_type, reference_id)
WHERE reference_type IS NOT NULL AND reference_id IS NOT NULL;

CREATE TABLE payment_webhook_receipts (
  id uuid PRIMARY KEY,
  gateway varchar(64) NOT NULL,
  external_event_id varchar(255) NOT NULL,
  payload jsonb NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  UNIQUE(gateway, external_event_id)
);

CREATE FUNCTION reject_ledger_mutation() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'ledger history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_transactions_immutable
BEFORE UPDATE OR DELETE ON ledger_transactions
FOR EACH ROW EXECUTE FUNCTION reject_ledger_mutation();

CREATE TRIGGER ledger_entries_immutable
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION reject_ledger_mutation();
