DROP INDEX IF EXISTS ix_plans_catalog_active;
DROP INDEX IF EXISTS ix_catalog_active_order;

ALTER TABLE plans
  DROP CONSTRAINT IF EXISTS plans_stock_mode_valid,
  DROP CONSTRAINT IF EXISTS plans_stock_quantity_valid,
  DROP COLUMN IF EXISTS metadata,
  DROP COLUMN IF EXISTS traffic_overage_price_minor,
  DROP COLUMN IF EXISTS setup_fee_minor,
  DROP COLUMN IF EXISTS stock_quantity;

ALTER TABLE products
  DROP CONSTRAINT IF EXISTS products_product_type_valid,
  DROP COLUMN IF EXISTS featured,
  DROP COLUMN IF EXISTS product_type;
