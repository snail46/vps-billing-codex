CREATE TABLE provider_health_checks (
  id uuid PRIMARY KEY,
  provider_id uuid NOT NULL REFERENCES providers(id),
  status varchar(32) NOT NULL CHECK (status IN ('healthy','degraded','unavailable')),
  version varchar(128),
  latency_ms integer NOT NULL CHECK (latency_ms >= 0),
  capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
  details jsonb NOT NULL DEFAULT '{}'::jsonb,
  error_code varchar(128),
  checked_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_provider_health_checks_provider
ON provider_health_checks(provider_id, checked_at DESC);

CREATE INDEX ix_providers_health_due
ON providers(last_health_check_at, id)
WHERE status <> 'disabled';
