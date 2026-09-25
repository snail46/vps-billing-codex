ALTER TABLE products
  ADD COLUMN product_type varchar(32) NOT NULL DEFAULT 'vps',
  ADD COLUMN featured boolean NOT NULL DEFAULT false,
  ADD CONSTRAINT products_product_type_valid CHECK (product_type IN ('vps','nat_vps'));

ALTER TABLE plans
  ADD COLUMN stock_quantity integer,
  ADD COLUMN setup_fee_minor bigint NOT NULL DEFAULT 0 CHECK (setup_fee_minor >= 0),
  ADD COLUMN traffic_overage_price_minor bigint NOT NULL DEFAULT 0 CHECK (traffic_overage_price_minor >= 0),
  ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD CONSTRAINT plans_stock_quantity_valid CHECK (stock_quantity IS NULL OR stock_quantity >= 0),
  ADD CONSTRAINT plans_stock_mode_valid CHECK (stock_mode IN ('automatic','manual'));

CREATE INDEX ix_catalog_active_order
ON products(featured DESC, sort_order, slug)
WHERE status = 'active';

CREATE INDEX ix_plans_catalog_active
ON plans(product_id, price_minor, slug)
WHERE status = 'active';
