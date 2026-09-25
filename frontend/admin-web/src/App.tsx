import { ApiRequestError, apiRequest, type AdminPrincipal, type AuthData, type ItemList, type TOTPSetup } from "@vps-billing/api-types";
import { type Locale, type MessageKey, useI18n } from "@vps-billing/i18n";
import { Alert, AppShell, Button, Card, EmptyState, ErrorState, Skeleton, StatusBadge } from "@vps-billing/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useEffect, useState } from "react";

type T = (key: MessageKey) => string;
type Item = Record<string, unknown>;
type Page =
  | "dashboard"
  | "users"
  | "products"
  | "orders"
  | "payments"
  | "ledger"
  | "subscriptions"
  | "instances"
  | "nodes"
  | "providers"
  | "operations"
  | "usage"
  | "dead-letters"
  | "tickets"
  | "audit"
  | "admins"
  | "roles"
  | "settings";

type DashboardData = {
  metrics: Item;
  critical_alerts: Item[];
  failed_operations: Item[];
  provider_health: Item[];
  node_health: Item[];
  capacity_warnings: Item[];
};

type AdminList = ItemList<Item> & { pagination: { total: number; limit: number; offset: number } };

const pages: { key: Page; permission: string }[] = [
  { key: "dashboard", permission: "health.read" },
  { key: "users", permission: "users.read" },
  { key: "products", permission: "products.read" },
  { key: "orders", permission: "orders.read" },
  { key: "payments", permission: "payments.read" },
  { key: "ledger", permission: "ledger.read" },
  { key: "subscriptions", permission: "subscriptions.read" },
  { key: "instances", permission: "instances.read" },
  { key: "nodes", permission: "nodes.read" },
  { key: "providers", permission: "providers.read" },
  { key: "operations", permission: "operations.read" },
  { key: "usage", permission: "usage.read" },
  { key: "dead-letters", permission: "operations.read" },
  { key: "tickets", permission: "tickets.read" },
  { key: "audit", permission: "audit.read" },
  { key: "admins", permission: "admins.read" },
  { key: "roles", permission: "roles.read" },
  { key: "settings", permission: "settings.read" },
];

const columns: Partial<Record<Page, string[]>> = {
  users: ["email", "status", "locale", "created_at"],
  products: ["slug", "status", "name_i18n", "created_at"],
  orders: ["order_no", "user_email", "status", "total_minor", "currency", "created_at"],
  payments: ["payment_no", "order_no", "user_email", "status", "amount_minor", "currency", "gateway", "created_at"],
  ledger: ["type", "reference_type", "description", "created_at"],
  subscriptions: ["user_email", "plan_slug", "status", "billing_cycle", "current_period_end"],
  instances: ["name", "user_email", "observed_state", "desired_state", "node_name", "provider_name"],
  nodes: ["name", "region", "status", "provider_name", "available_memory_mb", "last_seen_at"],
  providers: ["name", "provider_type", "status", "version", "last_health_check_at"],
  operations: ["type", "resource_type", "status", "phase", "progress", "error_code", "created_at"],
  usage: ["user_email", "instance_name", "status", "used_bytes", "overage_bytes", "amount_minor", "currency", "period_end"],
  "dead-letters": ["event_type", "aggregate_type", "attempts", "error_code", "dead_lettered_at"],
  tickets: ["ticket_no", "user_email", "subject", "status", "priority", "message_count", "updated_at"],
  audit: ["action", "resource_type", "actor_email", "request_id", "created_at"],
  admins: ["email", "display_name", "status", "two_factor_enabled", "roles"],
  roles: ["key", "permissions"],
  settings: ["key", "value", "is_secret", "updated_at"],
};

export function App() {
  const { locale, setLocale, t } = useI18n();
  const client = useQueryClient();
  const auth = useQuery({
    queryKey: ["admin-auth"],
    queryFn: () => apiRequest<AuthData<AdminPrincipal>>("/api/v1/admin/auth/me"),
    retry: false,
  });
  const [page, setPage] = useHashPage();

  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = t("admin.appName");
  }, [locale, t]);

  if (auth.isPending) {
    return (
      <Shell locale={locale} setLocale={setLocale} t={t}>
        <Skeleton label={t("common.loading")} />
      </Shell>
    );
  }

  if (auth.isError && (!(auth.error instanceof ApiRequestError) || auth.error.status !== 401)) {
    return (
      <Shell locale={locale} setLocale={setLocale} t={t}>
        <ErrorState title={errorText(auth.error, t)} retry={() => void auth.refetch()} retryLabel={t("common.retry")} />
      </Shell>
    );
  }

  if (!auth.data) {
    return <Login locale={locale} setLocale={setLocale} t={t} success={(data) => client.setQueryData(["admin-auth"], data)} />;
  }

  const allowed = pages.filter((item) => auth.data.principal.permissions.includes(item.permission));
  const selected = allowed.some((item) => item.key === page) ? page : allowed[0]?.key;

  return (
    <Shell
      locale={locale}
      setLocale={setLocale}
      t={t}
      actions={
        <>
          <AdminTOTP principal={auth.data.principal} csrf={auth.data.csrf_token} t={t} />
          <Logout csrf={auth.data.csrf_token} t={t} />
        </>
      }
    >
      <div className="admin-layout">
        <nav className="admin-nav" aria-label={t("admin.appName")}>
          {allowed.map((item) => (
            <button key={item.key} data-active={selected === item.key} onClick={() => setPage(item.key)}>
              {t(`admin.nav.${item.key}` as MessageKey)}
            </button>
          ))}
        </nav>
        <main className="admin-content">
          {selected ? (
            <PageView page={selected} principal={auth.data.principal} csrf={auth.data.csrf_token} locale={locale} t={t} />
          ) : (
            <ErrorState title={t("errors.permissionDenied")} />
          )}
        </main>
      </div>
    </Shell>
  );
}

export function Login({
  locale,
  setLocale,
  t,
  success,
}: {
  locale: Locale;
  setLocale: (v: Locale) => void;
  t: T;
  success: (v: AuthData<AdminPrincipal>) => void;
}) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<AuthData<AdminPrincipal>>("/api/v1/admin/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password, totp_code: code }),
      }),
    onSuccess: success,
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <Shell locale={locale} setLocale={setLocale} t={t}>
      <div className="auth-card">
        <Card>
          <h1>{t("auth.adminTitle")}</h1>
          <form className="auth-form" onSubmit={submit}>
            <label className="form-field">
              {t("auth.email")}
              <input
                className="form-input"
                type="email"
                autoComplete="username"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </label>
            <label className="form-field">
              {t("auth.password")}
              <input
                className="form-input"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
            <label className="form-field">
              {t("auth.totp")}
              <input
                className="form-input"
                inputMode="numeric"
                pattern="[0-9]{6}"
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
            </label>
            {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
            <Button type="submit" disabled={mutation.isPending}>
              {t(mutation.isPending ? "auth.pending" : "auth.login")}
            </Button>
          </form>
        </Card>
      </div>
    </Shell>
  );
}

function PageView({
  page,
  principal,
  csrf,
  locale,
  t,
}: {
  page: Page;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  if (page === "dashboard") return <Dashboard locale={locale} t={t} />;
  return <ResourcePage page={page} principal={principal} csrf={csrf} locale={locale} t={t} />;
}

function Dashboard({ locale, t }: { locale: Locale; t: T }) {
  const query = useQuery({
    queryKey: ["admin", "dashboard"],
    queryFn: () => apiRequest<DashboardData>("/api/v1/admin/dashboard"),
    refetchInterval: 30000,
  });

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const d = query.data;
  return (
    <section>
      <h1 className="page-title">{t("admin.nav.dashboard")}</h1>
      <h2>{t("admin.criticalAlerts")}</h2>
      {d.critical_alerts.length ? (
        <DenseTable items={d.critical_alerts} fields={["kind", "status", "error_code", "created_at"]} locale={locale} t={t} />
      ) : (
        <Alert tone="success">{t("admin.noCriticalAlerts")}</Alert>
      )}
      <div className="metric-grid admin-metrics">
        {Object.entries(d.metrics).map(([key, value]) => (
          <div className="metric" key={key}>
            <span>{t(`admin.metric.${key}` as MessageKey)}</span>
            <strong>{String(value)}</strong>
          </div>
        ))}
      </div>
      <DashboardPanel title={t("admin.failedOperations")} items={d.failed_operations} fields={["type", "status", "phase", "error_code"]} locale={locale} t={t} />
      <DashboardPanel title={t("admin.providerHealth")} items={d.provider_health} fields={["name", "provider_type", "status", "last_health_check_at"]} locale={locale} t={t} />
      <DashboardPanel title={t("admin.nodeHealth")} items={d.node_health} fields={["name", "region", "status", "last_seen_at"]} locale={locale} t={t} />
      <DashboardPanel title={t("admin.capacityWarnings")} items={d.capacity_warnings} fields={["name", "cpu_percent", "memory_percent", "disk_percent"]} locale={locale} t={t} />
    </section>
  );
}

function DashboardPanel({
  title,
  items,
  fields,
  locale,
  t,
}: {
  title: string;
  items: Item[];
  fields: string[];
  locale: Locale;
  t: T;
}) {
  return (
    <section className="admin-section">
      <h2>{title}</h2>
      {items.length ? <DenseTable items={items} fields={fields} locale={locale} t={t} /> : <EmptyState title={t("common.empty")} />}
    </section>
  );
}

function ResourcePage({
  page,
  principal,
  csrf,
  locale,
  t,
}: {
  page: Page;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [offset, setOffset] = useState(0);
  const limit = 25;
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (search) params.set("q", search);
  if (status) params.set("status", status);

  const query = useQuery({
    queryKey: ["admin", page, search, status, offset],
    queryFn: () => apiRequest<AdminList>(`/api/v1/admin/${page}?${params}`),
  });
  const [selected, setSelected] = useState<Item | null>(null);

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    if (query.error instanceof ApiRequestError && query.error.status === 403) {
      return <ErrorState title={t("errors.permissionDenied")} />;
    }
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const pagination = query.data.pagination;

  return (
    <section>
      <div className="card-heading">
        <h1 className="page-title">{t(`admin.nav.${page}` as MessageKey)}</h1>
        <ActionForm page={page} principal={principal} csrf={csrf} t={t} />
      </div>
      <div className="button-row">
        <label className="form-field">
          {t("admin.search")}
          <input
            className="form-input"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setOffset(0);
            }}
          />
        </label>
        <label className="form-field">
          {t("admin.field.status")}
          <input
            className="form-input"
            value={status}
            onChange={(event) => {
              setStatus(event.target.value);
              setOffset(0);
            }}
          />
        </label>
      </div>
      {query.data.items.length ? (
        <DenseTable items={query.data.items} fields={columns[page] ?? []} locale={locale} t={t} select={setSelected} />
      ) : (
        <EmptyState title={t("common.empty")} />
      )}
      <div className="button-row">
        <button
          className="button button--secondary"
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - limit))}
        >
          {t("admin.previous")}
        </button>
        <span>
          {offset + 1}–{Math.min(offset + limit, pagination.total)} / {pagination.total}
        </span>
        <button
          className="button button--secondary"
          disabled={offset + limit >= pagination.total}
          onClick={() => setOffset(offset + limit)}
        >
          {t("admin.next")}
        </button>
      </div>
      {selected && (
        <Detail
          page={page}
          item={selected}
          principal={principal}
          csrf={csrf}
          locale={locale}
          t={t}
          close={() => setSelected(null)}
        />
      )}
    </section>
  );
}

function DenseTable({
  items,
  fields,
  locale,
  t,
  select,
}: {
  items: Item[];
  fields: string[];
  locale: Locale;
  t: T;
  select?: (i: Item) => void;
}) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            {fields.map((field) => (
              <th key={field}>{t(`admin.field.${field}` as MessageKey)}</th>
            ))}
            {select && <th>{t("common.actions")}</th>}
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) => (
            <tr key={String(item.id ?? index)}>
              {fields.map((field) => (
                <td key={field}>{renderValue(field, item[field], locale, t)}</td>
              ))}
              {select && (
                <td>
                  <button className="link-button" onClick={() => select(item)}>
                    {t("common.view")}
                  </button>
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function renderValue(field: string, value: unknown, locale: Locale, t: T) {
  if (value === null || value === undefined || value === "") return "—";
  if (field.includes("status") || field === "observed_state" || field === "desired_state") {
    return <StatusBadge status={String(value)} label={statusText(String(value), t)} />;
  }
  if (field.endsWith("_at")) {
    const date = new Date(String(value));
    return Number.isNaN(date.valueOf()) ? String(value) : date.toLocaleString(locale);
  }
  if (field.endsWith("_minor")) {
    return new Intl.NumberFormat(locale).format(Number(value));
  }
  if (typeof value === "object") {
    if (Array.isArray(value)) return value.join(", ");
    if (value && typeof value === "object" && locale in (value as Record<string, string>)) {
      return (value as Record<string, string>)[locale] || (value as Record<string, string>)["en-US"] || JSON.stringify(value);
    }
    return JSON.stringify(value);
  }
  if (typeof value === "boolean") {
    return t(value ? "common.yes" : "common.no");
  }
  return String(value);
}

function Detail({
  page,
  item,
  principal,
  csrf,
  locale,
  t,
  close,
}: {
  page: Page;
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
  close: () => void;
}) {
  const id = String(item.id ?? "");
  const hasDetail = page === "instances" || page === "operations" || page === "providers";
  const query = useQuery({
    queryKey: ["admin", page, id],
    queryFn: () => apiRequest<Item>(`/api/v1/admin/${page}/${id}`),
    enabled: hasDetail && Boolean(id),
  });

  let content: React.ReactNode;

  if (page === "tickets") {
    content = <TicketActions item={item} principal={principal} csrf={csrf} locale={locale} t={t} />;
  } else if (page === "users") {
    content = <UserActions item={item} principal={principal} csrf={csrf} locale={locale} t={t} />;
  } else if (page === "admins") {
    content = <AdminRoleActions item={item} principal={principal} csrf={csrf} locale={locale} t={t} />;
  } else if (page === "payments") {
    content = <PaymentActions item={item} principal={principal} csrf={csrf} locale={locale} t={t} />;
  } else if (page === "dead-letters") {
    content = <DeadLetterActions item={item} principal={principal} csrf={csrf} locale={locale} t={t} />;
  } else if (page === "products") {
    content = <ProductDetail item={item} principal={principal} csrf={csrf} locale={locale} t={t} />;
  } else if (page === "orders") {
    content = <OrderDetail item={item} locale={locale} t={t} />;
  } else if (page === "subscriptions") {
    content = <SubscriptionDetail item={item} locale={locale} t={t} />;
  } else if (page === "audit") {
    content = <AuditDetail item={item} locale={locale} t={t} />;
  } else if (page === "settings") {
    content = <SettingDetail item={item} principal={principal} csrf={csrf} locale={locale} t={t} />;
  } else if (hasDetail) {
    if (query.isPending) {
      content = <Skeleton label={t("common.loading")} />;
    } else if (query.isError) {
      content = <ErrorState title={errorText(query.error, t)} />;
    } else if (query.data) {
      if (page === "instances") {
        content = <InstanceDetail item={query.data} locale={locale} t={t} />;
      } else if (page === "operations") {
        content = <OperationDetail item={query.data} principal={principal} csrf={csrf} locale={locale} t={t} />;
      } else if (page === "providers") {
        content = <ProviderDetail item={query.data} locale={locale} t={t} />;
      }
    }
  } else {
    content = <GenericDetail item={item} locale={locale} t={t} />;
  }

  return (
    <div className="dialog-backdrop">
      <section className="dialog admin-detail" role="dialog" aria-modal="true">
        <div className="card-heading">
          <h2>{t("admin.detail")}</h2>
          <button className="button button--secondary" onClick={close}>
            {t("common.close")}
          </button>
        </div>
        {content}
        <small style={{ marginTop: 12, display: "block", color: "#667085" }}>
          {new Date().toLocaleString(locale)}
        </small>
      </section>
    </div>
  );
}

function InstanceDetail({ item, locale, t }: { item: Item; locale: Locale; t: T }) {
  const networks = Array.isArray(item.networks) ? (item.networks as Item[]) : [];
  const operations = Array.isArray(item.operations) ? (item.operations as Item[]) : [];

  return (
    <div>
      <h3>{t("admin.instance.overview")}</h3>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.name")}</td>
            <td><strong>{String(item.name ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.field.hostname")}</td>
            <td>{String(item.hostname ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.user_email")}</td>
            <td>{String(item.user_email ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.plan_slug")}</td>
            <td>{String(item.plan_slug ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.observed_state")}</td>
            <td><StatusBadge status={String(item.observed_state ?? "unknown")} label={statusText(String(item.observed_state ?? "unknown"), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.desired_state")}</td>
            <td><StatusBadge status={String(item.desired_state ?? "unknown")} label={statusText(String(item.desired_state ?? "unknown"), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.specs")}</td>
            <td>
              {item.cpu_cores ? `${item.cpu_cores} vCPU / ` : ""}
              {item.memory_mb ? `${item.memory_mb} MB RAM / ` : ""}
              {item.disk_gb ? `${item.disk_gb} GB Disk` : ""}
            </td>
          </tr>
          <tr>
            <td>{t("admin.field.created_at")}</td>
            <td>{renderValue("created_at", item.created_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {networks.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h4>{t("admin.field.networks")}</h4>
          <DenseTable
            items={networks}
            fields={["network_type", "ip_address", "mac_address", "created_at"]}
            locale={locale}
            t={t}
          />
        </div>
      )}

      {operations.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h4>{t("admin.nav.operations")}</h4>
          <DenseTable
            items={operations}
            fields={["type", "status", "phase", "progress", "created_at"]}
            locale={locale}
            t={t}
          />
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function OperationDetail({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const steps = Array.isArray(item.steps) ? (item.steps as Item[]) : [];
  const attempts = Array.isArray(item.attempts) ? (item.attempts as Item[]) : [];
  const progressNum = Number(item.progress ?? 0);

  return (
    <div>
      <div className="card-heading">
        <h3>{String(item.type ?? t("admin.nav.operations"))}</h3>
        <OperationActions item={item} principal={principal} csrf={csrf} t={t} />
      </div>

      <div className="progress-bar-wrap" style={{ margin: "12px 0 16px" }}>
        <div
          className={`progress-bar-fill ${item.status === "failed" ? "progress-bar-fill--danger" : ""}`}
          style={{ width: `${Math.min(100, Math.max(0, progressNum))}%` }}
        />
      </div>

      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.phase")}</td>
            <td>{String(item.phase ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.progress")}</td>
            <td>{progressNum}%</td>
          </tr>
          <tr>
            <td>{t("admin.field.resource_type")}</td>
            <td>{String(item.resource_type ?? "—")} ({String(item.resource_id ?? "—")})</td>
          </tr>
          {Boolean(item.error_code) && (
            <tr>
              <td>{t("admin.field.error_code")}</td>
              <td><span style={{ color: "#d92d20", fontWeight: 600 }}>{String(item.error_code)}</span></td>
            </tr>
          )}
          <tr>
            <td>{t("admin.field.provider")}</td>
            <td>{String(item.provider_name ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.node")}</td>
            <td>{String(item.node_name ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.created_at")}</td>
            <td>{renderValue("created_at", item.created_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {steps.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h4>{t("admin.operation.steps")}</h4>
          <DenseTable
            items={steps}
            fields={["step_key", "step_order", "status", "progress", "attempt_count", "error_code"]}
            locale={locale}
            t={t}
          />
        </div>
      )}

      {attempts.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h4>{t("admin.operation.attempts")}</h4>
          <DenseTable
            items={attempts}
            fields={["attempt", "status", "error_code", "started_at", "finished_at"]}
            locale={locale}
            t={t}
          />
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function ProviderDetail({ item, locale, t }: { item: Item; locale: Locale; t: T }) {
  const nodes = Array.isArray(item.nodes) ? (item.nodes as Item[]) : [];
  const healthHistory = Array.isArray(item.health_history) ? (item.health_history as Item[]) : [];

  return (
    <div>
      <h3>{t("admin.provider.overview")}</h3>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.name")}</td>
            <td><strong>{String(item.name ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.field.provider_type")}</td>
            <td>{String(item.provider_type ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.version")}</td>
            <td>{String(item.version ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.last_health_check_at")}</td>
            <td>{renderValue("last_health_check_at", item.last_health_check_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {nodes.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h4>{t("admin.field.nodes")}</h4>
          <DenseTable
            items={nodes}
            fields={["name", "region", "status", "last_seen_at"]}
            locale={locale}
            t={t}
          />
        </div>
      )}

      {healthHistory.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h4>{t("admin.field.health_history")}</h4>
          <DenseTable
            items={healthHistory}
            fields={["status", "latency_ms", "checked_at"]}
            locale={locale}
            t={t}
          />
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function ProductDetail({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const client = useQueryClient();
  const [status, setStatus] = useState(String(item.status ?? "draft"));
  const plans = Array.isArray(item.plans) ? (item.plans as Item[]) : [];

  const updateMutation = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/products/${item.id}`, {
        method: "PATCH",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({
          slug: item.slug,
          name_i18n: item.name_i18n ?? { "zh-CN": item.slug, "en-US": item.slug },
          description_i18n: item.description_i18n ?? { "zh-CN": "", "en-US": "" },
          status,
          sort_order: Number(item.sort_order ?? 0),
          product_type: String(item.product_type ?? "vps"),
          featured: Boolean(item.featured),
        }),
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["admin", "products"] });
    },
  });

  return (
    <div>
      <div className="card-heading">
        <h3>{renderValue("name_i18n", item.name_i18n, locale, t)}</h3>
        <StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} />
      </div>

      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.slug")}</td>
            <td>{String(item.slug ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.created_at")}</td>
            <td>{renderValue("created_at", item.created_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {principal.permissions.includes("products.manage") && (
        <form
          className="auth-form"
          style={{ marginTop: 16 }}
          onSubmit={(e) => {
            e.preventDefault();
            updateMutation.mutate();
          }}
        >
          <h4>{t("admin.catalog.editStatus")}</h4>
          <label className="form-field">
            {t("admin.field.status")}
            <select
              className="form-input"
              value={status}
              onChange={(e) => setStatus(e.target.value)}
            >
              <option value="draft">{t("admin.catalog.draft")}</option>
              <option value="active">{t("admin.catalog.active")}</option>
              <option value="archived">{t("admin.catalog.archived")}</option>
            </select>
          </label>
          {updateMutation.isError && <Alert tone="danger">{errorText(updateMutation.error, t)}</Alert>}
          {updateMutation.isSuccess && <Alert tone="success">{t("admin.catalog.statusUpdated")}</Alert>}
          <Button type="submit" disabled={updateMutation.isPending}>
            {t(updateMutation.isPending ? "common.submitting" : "common.save")}
          </Button>
        </form>
      )}

      {plans.length > 0 && (
        <div style={{ marginTop: 20 }}>
          <h4>{t("admin.catalog.plan")}</h4>
          <DenseTable
            items={plans}
            fields={["slug", "status", "price_minor", "currency", "billing_cycle"]}
            locale={locale}
            t={t}
          />
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function OrderDetail({ item, locale, t }: { item: Item; locale: Locale; t: T }) {
  return (
    <div>
      <h3>{t("admin.order.overview")}</h3>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.order_no")}</td>
            <td><strong>{String(item.order_no ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.field.user_email")}</td>
            <td>{String(item.user_email ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.total_minor")}</td>
            <td>{renderValue("total_minor", item.total_minor, locale, t)} {String(item.currency ?? "USD")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.created_at")}</td>
            <td>{renderValue("created_at", item.created_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function SubscriptionDetail({ item, locale, t }: { item: Item; locale: Locale; t: T }) {
  return (
    <div>
      <h3>{t("admin.subscription.overview")}</h3>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.plan_slug")}</td>
            <td><strong>{String(item.plan_slug ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.field.user_email")}</td>
            <td>{String(item.user_email ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.billing_cycle")}</td>
            <td>{String(item.billing_cycle ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.current_period_end")}</td>
            <td>{renderValue("current_period_end", item.current_period_end, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function AuditDetail({ item, locale, t }: { item: Item; locale: Locale; t: T }) {
  return (
    <div>
      <h3>{t("admin.audit.details")}</h3>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.action")}</td>
            <td><strong>{String(item.action ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.audit.actor")}</td>
            <td>{String(item.actor_email ?? item.actor_id ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.resource_type")}</td>
            <td>{String(item.resource_type ?? "—")} ({String(item.resource_id ?? "—")})</td>
          </tr>
          <tr>
            <td>{t("admin.field.request_id")}</td>
            <td><code>{String(item.request_id ?? "—")}</code></td>
          </tr>
          <tr>
            <td>{t("admin.field.created_at")}</td>
            <td>{renderValue("created_at", item.created_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {Boolean(item.before || item.after) && (
        <div style={{ marginTop: 16 }}>
          <h4>{t("admin.audit.changes")}</h4>
          {Boolean(item.before) && (
            <div>
              <span style={{ fontWeight: 600, color: "#667085" }}>{t("admin.field.before")}:</span>
              <pre className="diagnostic">{JSON.stringify(item.before, null, 2)}</pre>
            </div>
          )}
          {Boolean(item.after) && (
            <div style={{ marginTop: 8 }}>
              <span style={{ fontWeight: 600, color: "#667085" }}>{t("admin.field.after")}:</span>
              <pre className="diagnostic">{JSON.stringify(item.after, null, 2)}</pre>
            </div>
          )}
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function SettingDetail({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const client = useQueryClient();
  const [val, setVal] = useState(typeof item.value === "object" ? JSON.stringify(item.value) : String(item.value ?? ""));
  const [secret, setSecret] = useState(Boolean(item.is_secret));

  const updateMutation = useMutation({
    mutationFn: () => {
      let parsedValue: unknown = val;
      try {
        parsedValue = JSON.parse(val);
      } catch {
        // use string as-is
      }
      return apiRequest<Item>(`/api/v1/admin/settings/${encodeURIComponent(String(item.key))}`, {
        method: "PUT",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ value: parsedValue, is_secret: secret }),
      });
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["admin", "settings"] });
    },
  });

  return (
    <div>
      <h3>{t("admin.nav.settings")}</h3>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.key")}</td>
            <td><code>{String(item.key ?? "—")}</code></td>
          </tr>
          <tr>
            <td>{t("admin.field.is_secret")}</td>
            <td>{renderValue("is_secret", item.is_secret, locale, t)}</td>
          </tr>
          <tr>
            <td>{t("admin.field.updated_at")}</td>
            <td>{renderValue("updated_at", item.updated_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {principal.permissions.includes("settings.manage") && (
        <form
          className="auth-form"
          style={{ marginTop: 16 }}
          onSubmit={(e) => {
            e.preventDefault();
            updateMutation.mutate();
          }}
        >
          <h4>{t("admin.updateSetting")}</h4>
          <Field label={t("admin.field.value")} value={val} set={setVal} />
          <label className="check-line">
            <input type="checkbox" checked={secret} onChange={(e) => setSecret(e.target.checked)} />
            {t("admin.field.is_secret")}
          </label>
          {updateMutation.isError && <Alert tone="danger">{errorText(updateMutation.error, t)}</Alert>}
          <Button type="submit" disabled={updateMutation.isPending}>
            {t(updateMutation.isPending ? "common.submitting" : "common.save")}
          </Button>
        </form>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function GenericDetail({ item, locale, t }: { item: Item; locale: Locale; t: T }) {
  return (
    <div>
      <table className="admin-kv-table">
        <tbody>
          {Object.entries(item).slice(0, 10).map(([k, v]) => (
            <tr key={k}>
              <td>{k}</td>
              <td>{renderValue(k, v, locale, t)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <details open style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function PaymentActions({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const client = useQueryClient();
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/payments/${item.id}/refunds`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({ amount_minor: Number(amount), reason }),
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["admin", "payments"] }),
  });

  return (
    <div>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.payment_no")}</td>
            <td>{String(item.payment_no ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.order_no")}</td>
            <td>{String(item.order_no ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.amount_minor")}</td>
            <td>{renderValue("amount_minor", item.amount_minor, locale, t)} {String(item.currency ?? "USD")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
        </tbody>
      </table>

      {principal.permissions.includes("payments.refund") && (
        <form
          className="auth-form"
          style={{ marginTop: 16 }}
          onSubmit={(event) => {
            event.preventDefault();
            mutation.mutate();
          }}
        >
          <h4>{t("admin.payment.refund")}</h4>
          <Field label={t("admin.field.amount_minor")} value={amount} set={setAmount} type="number" />
          <Field label={t("admin.field.reason")} value={reason} set={setReason} />
          {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
          <Button type="submit" disabled={mutation.isPending}>
            {t(mutation.isPending ? "common.submitting" : "admin.payment.refund")}
          </Button>
        </form>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function DeadLetterActions({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/dead-letters/${item.id}/replay`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["admin", "dead-letters"] }),
  });

  return (
    <div>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.id")}</td>
            <td><code>{String(item.id ?? "—")}</code></td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.attempt")}</td>
            <td>{String(item.attempts ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.created_at")}</td>
            <td>{renderValue("created_at", item.dead_lettered_at ?? item.created_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {principal.permissions.includes("outbox.replay") && (
        <div style={{ marginTop: 16 }}>
          {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
          <Button disabled={mutation.isPending} onClick={() => mutation.mutate()}>
            {t(mutation.isPending ? "common.submitting" : "admin.deadLetter.replay")}
          </Button>
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function OperationActions({
  item,
  principal,
  csrf,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  t: T;
}) {
  const client = useQueryClient();
  const retry = useMutation({
    mutationFn: () =>
      apiRequest<{ operation_id: string }>(`/api/v1/admin/operations/${item.id}/retry`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() },
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["admin", "operations"] }),
  });
  const cancel = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/operations/${item.id}/cancel`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["admin", "operations"] });
      void client.invalidateQueries({ queryKey: ["admin", "operations", String(item.id)] });
    },
  });

  if (!principal.permissions.includes("operations.retry")) return null;
  const status = String(item.status);
  return (
    <div className="button-row">
      {["failed", "cancelled"].includes(status) && (
        <Button disabled={retry.isPending} onClick={() => retry.mutate()}>
          {t(retry.isPending ? "common.submitting" : "admin.operation.retry")}
        </Button>
      )}
      {["queued", "retrying"].includes(status) && (
        <button
          className="button button--danger"
          type="button"
          disabled={cancel.isPending}
          onClick={() => cancel.mutate()}
        >
          {t(cancel.isPending ? "common.submitting" : "admin.operation.cancel")}
        </button>
      )}
      {(retry.isError || cancel.isError) && (
        <Alert tone="danger">{errorText(retry.error ?? cancel.error, t)}</Alert>
      )}
    </div>
  );
}

function UserActions({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const client = useQueryClient();
  const [status, setStatus] = useState(String(item.status));
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/users/${item.id}/status`, {
        method: "PATCH",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ status }),
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["admin", "users"] }),
  });

  return (
    <div>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.email")}</td>
            <td><strong>{String(item.email ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.locale")}</td>
            <td>{String(item.locale ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.created_at")}</td>
            <td>{renderValue("created_at", item.created_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {principal.permissions.includes("users.suspend") && (
        <div className="auth-form" style={{ marginTop: 16 }}>
          <label className="form-field">
            {t("admin.field.status")}
            <select className="form-input" value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="active">{statusText("active", t)}</option>
              <option value="suspended">{statusText("suspended", t)}</option>
            </select>
          </label>
          {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            {t(mutation.isPending ? "common.submitting" : "admin.updateUser")}
          </Button>
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function AdminRoleActions({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const choices = ["super_admin", "operations", "finance", "support", "read_only"];
  const client = useQueryClient();
  const [roles, setRoles] = useState<string[]>(Array.isArray(item.roles) ? item.roles.map(String) : []);
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/admins/${item.id}/roles`, {
        method: "PUT",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ roles }),
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["admin", "admins"] }),
  });

  return (
    <div>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.email")}</td>
            <td><strong>{String(item.email ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.field.display_name")}</td>
            <td>{String(item.display_name ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.two_factor_enabled")}</td>
            <td>{renderValue("two_factor_enabled", item.two_factor_enabled, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      {principal.permissions.includes("admins.manage") && (
        <div className="auth-form" style={{ marginTop: 16 }}>
          <fieldset>
            <legend>{t("admin.field.roles")}</legend>
            {choices.map((role) => (
              <label className="check-line" key={role}>
                <input
                  type="checkbox"
                  checked={roles.includes(role)}
                  onChange={(e) =>
                    setRoles(e.target.checked ? [...roles, role] : roles.filter((v) => v !== role))
                  }
                />{" "}
                {t(`roles.${role}` as MessageKey)}
              </label>
            ))}
          </fieldset>
          {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
          <Button onClick={() => mutation.mutate()} disabled={!roles.length || mutation.isPending}>
            {t(mutation.isPending ? "common.submitting" : "admin.updateRoles")}
          </Button>
        </div>
      )}

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function TicketActions({
  item,
  principal,
  csrf,
  locale,
  t,
}: {
  item: Item;
  principal: AdminPrincipal;
  csrf: string;
  locale: Locale;
  t: T;
}) {
  const client = useQueryClient();
  const [message, setMessage] = useState("");
  const [status, setStatus] = useState(String(item.status));

  const reply = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/tickets/${item.id}/messages`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ message }),
      }),
    onSuccess: () => {
      setMessage("");
      void client.invalidateQueries({ queryKey: ["admin", "tickets"] });
    },
  });

  const update = useMutation({
    mutationFn: () =>
      apiRequest<Item>(`/api/v1/admin/tickets/${item.id}/status`, {
        method: "PATCH",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ status }),
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["admin", "tickets"] }),
  });

  return (
    <div>
      <table className="admin-kv-table">
        <tbody>
          <tr>
            <td>{t("admin.field.ticket_no")}</td>
            <td><strong>{String(item.ticket_no ?? "—")}</strong></td>
          </tr>
          <tr>
            <td>{t("admin.field.user_email")}</td>
            <td>{String(item.user_email ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.subject")}</td>
            <td>{String(item.subject ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.status")}</td>
            <td><StatusBadge status={String(item.status ?? "")} label={statusText(String(item.status ?? ""), t)} /></td>
          </tr>
          <tr>
            <td>{t("admin.field.priority")}</td>
            <td>{String(item.priority ?? "—")}</td>
          </tr>
          <tr>
            <td>{t("admin.field.updated_at")}</td>
            <td>{renderValue("updated_at", item.updated_at, locale, t)}</td>
          </tr>
        </tbody>
      </table>

      <div className="auth-form" style={{ marginTop: 16 }}>
        {principal.permissions.includes("tickets.reply") && (
          <>
            <label className="form-field">
              {t("admin.reply")}
              <textarea
                className="form-input textarea"
                value={message}
                onChange={(e) => setMessage(e.target.value)}
              />
            </label>
            <Button onClick={() => reply.mutate()} disabled={!message || reply.isPending}>
              {t(reply.isPending ? "common.submitting" : "admin.sendReply")}
            </Button>
          </>
        )}
        {principal.permissions.includes("tickets.manage") && (
          <>
            <label className="form-field" style={{ marginTop: 12 }}>
              {t("admin.field.status")}
              <select className="form-input" value={status} onChange={(e) => setStatus(e.target.value)}>
                {["open", "waiting_user", "waiting_support", "resolved", "closed"].map((v) => (
                  <option value={v} key={v}>
                    {statusText(v, t)}
                  </option>
                ))}
              </select>
            </label>
            <Button onClick={() => update.mutate()} disabled={update.isPending}>
              {t(update.isPending ? "common.submitting" : "common.save")}
            </Button>
          </>
        )}
        {(reply.isError || update.isError) && (
          <Alert tone="danger">{errorText(reply.error ?? update.error, t)}</Alert>
        )}
      </div>

      <details style={{ marginTop: 16 }}>
        <summary style={{ cursor: "pointer", color: "#155eef", fontWeight: 600 }}>
          {t("admin.operation.rawDiagnostics")}
        </summary>
        <pre className="diagnostic">{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

function ActionForm({
  page,
  principal,
  csrf,
  t,
}: {
  page: Page;
  principal: AdminPrincipal;
  csrf: string;
  t: T;
}) {
  if (page === "products" && principal.permissions.includes("products.manage")) {
    return <CatalogForm csrf={csrf} t={t} />;
  }
  if (page === "ledger" && principal.permissions.includes("ledger.adjust")) {
    return <WalletAdjustment csrf={csrf} t={t} />;
  }
  if (page === "settings" && principal.permissions.includes("settings.manage")) {
    return <SettingForm csrf={csrf} t={t} />;
  }
  return null;
}

function CatalogForm({ csrf, t }: { csrf: string; t: T }) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<"product" | "plan">("product");
  const [productType, setProductType] = useState<"vps" | "nat_vps">("vps");
  const [productID, setProductID] = useState("");
  const [slug, setSlug] = useState("");
  const [zh, setZh] = useState("");
  const [en, setEn] = useState("");
  const [nodeGroupID, setNodeGroupID] = useState("");
  const [cpu, setCPU] = useState("1");
  const [memory, setMemory] = useState("1024");
  const [disk, setDisk] = useState("20");
  const [traffic, setTraffic] = useState("1000");
  const [bandwidth, setBandwidth] = useState("100");
  const [ipv4, setIPv4] = useState("1");
  const [ipv6, setIPv6] = useState("0");
  const [natPorts, setNATPorts] = useState("0");
  const [price, setPrice] = useState("0");

  const mutation = useMutation({
    mutationFn: () =>
      mode === "product"
        ? apiRequest<Item>("/api/v1/admin/products", {
            method: "POST",
            headers: { "X-CSRF-Token": csrf },
            body: JSON.stringify({
              slug,
              name_i18n: { "zh-CN": zh, "en-US": en },
              description_i18n: { "zh-CN": "", "en-US": "" },
              status: "draft",
              sort_order: 0,
              product_type: productType,
              featured: false,
            }),
          })
        : apiRequest<Item>(`/api/v1/admin/products/${productID}/plans`, {
            method: "POST",
            headers: { "X-CSRF-Token": csrf },
            body: JSON.stringify({
              slug,
              name_i18n: { "zh-CN": zh, "en-US": en },
              status: "draft",
              node_group_id: nodeGroupID || null,
              cpu_cores: Number(cpu),
              memory_mb: Number(memory),
              disk_gb: Number(disk),
              traffic_gb: traffic ? Number(traffic) : null,
              bandwidth_mbps: bandwidth ? Number(bandwidth) : null,
              ipv4_count: Number(ipv4),
              ipv6_count: Number(ipv6),
              nat_port_count: Number(natPorts),
              virtualization: "kvm",
              billing_cycle: "monthly",
              price_minor: Number(price),
              currency: "USD",
              stock_mode: "automatic",
              default_image_id: "ubuntu-24.04",
              stock_quantity: null,
              setup_fee_minor: 0,
              traffic_overage_price_minor: 0,
            }),
          }),
    onSuccess: () => {
      setOpen(false);
      void client.invalidateQueries({ queryKey: ["admin", "products"] });
    },
  });

  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("admin.catalog.create")}</Button>
      {open && (
        <div className="dialog-backdrop">
          <form
            className="dialog auth-form"
            onSubmit={(event) => {
              event.preventDefault();
              mutation.mutate();
            }}
          >
            <h2>{t("admin.catalog.create")}</h2>
            <label className="form-field">
              {t("admin.catalog.kind")}
              <select
                className="form-input"
                value={mode}
                onChange={(event) => setMode(event.target.value as "product" | "plan")}
              >
                <option value="product">{t("admin.catalog.product")}</option>
                <option value="plan">{t("admin.catalog.plan")}</option>
              </select>
            </label>
            {mode === "product" && (
              <label className="form-field">
                {t("catalog.type")}
                <select
                  className="form-input"
                  value={productType}
                  onChange={(event) => setProductType(event.target.value as typeof productType)}
                >
                  <option value="vps">{t("catalog.type.vps")}</option>
                  <option value="nat_vps">{t("catalog.type.nat_vps")}</option>
                </select>
              </label>
            )}
            {mode === "plan" && <Field label={t("admin.field.product_id")} value={productID} set={setProductID} />}
            <Field label={t("admin.field.slug")} value={slug} set={setSlug} />
            <Field label={t("admin.catalog.nameZh")} value={zh} set={setZh} />
            <Field label={t("admin.catalog.nameEn")} value={en} set={setEn} />
            {mode === "plan" && (
              <>
                <Field label={t("admin.field.node_group_id")} value={nodeGroupID} set={setNodeGroupID} />
                <div className="detail-grid">
                  <Field label={t("admin.field.cpu_cores")} value={cpu} set={setCPU} type="number" />
                  <Field label={t("admin.field.memory_mb")} value={memory} set={setMemory} type="number" />
                  <Field label={t("admin.field.disk_gb")} value={disk} set={setDisk} type="number" />
                  <Field label={t("admin.field.traffic_gb")} value={traffic} set={setTraffic} type="number" />
                  <Field label={t("admin.field.bandwidth_mbps")} value={bandwidth} set={setBandwidth} type="number" />
                  <Field label={t("admin.field.ipv4_count")} value={ipv4} set={setIPv4} type="number" />
                  <Field label={t("admin.field.ipv6_count")} value={ipv6} set={setIPv6} type="number" />
                  <Field label={t("admin.field.nat_port_count")} value={natPorts} set={setNATPorts} type="number" />
                  <Field label={t("admin.field.price_minor")} value={price} set={setPrice} type="number" />
                </div>
              </>
            )}
            {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
            <div className="button-row">
              <Button type="submit" disabled={mutation.isPending}>
                {t(mutation.isPending ? "common.submitting" : "common.save")}
              </Button>
              <button type="button" className="button button--secondary" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        </div>
      )}
    </>
  );
}

function WalletAdjustment({ csrf, t }: { csrf: string; t: T }) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [userID, setUserID] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [amount, setAmount] = useState("");
  const [description, setDescription] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<Item>("/api/v1/admin/ledger/adjustments", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({
          user_id: userID,
          currency,
          amount_minor: Number(amount),
          description,
        }),
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["admin", "ledger"] }),
  });

  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("admin.adjustWallet")}</Button>
      {open && (
        <div className="dialog-backdrop">
          <form
            className="dialog auth-form"
            onSubmit={(e) => {
              e.preventDefault();
              mutation.mutate();
            }}
          >
            <h2>{t("admin.adjustWallet")}</h2>
            <Field label={t("admin.field.user_id")} value={userID} set={setUserID} />
            <Field label={t("admin.field.currency")} value={currency} set={setCurrency} />
            <Field label={t("admin.field.amount_minor")} value={amount} set={setAmount} type="number" />
            <Field label={t("admin.field.description")} value={description} set={setDescription} />
            {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
            <div className="button-row">
              <Button type="submit" disabled={mutation.isPending}>
                {t(mutation.isPending ? "common.submitting" : "common.save")}
              </Button>
              <button type="button" className="button button--secondary" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        </div>
      )}
    </>
  );
}

function SettingForm({ csrf, t }: { csrf: string; t: T }) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [secret, setSecret] = useState(false);

  const mutation = useMutation({
    mutationFn: () => {
      let parsedValue: unknown = value;
      try {
        parsedValue = JSON.parse(value);
      } catch {
        // use string as-is
      }
      return apiRequest<Item>(`/api/v1/admin/settings/${encodeURIComponent(key)}`, {
        method: "PUT",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ value: parsedValue, is_secret: secret }),
      });
    },
    onSuccess: () => {
      setOpen(false);
      void client.invalidateQueries({ queryKey: ["admin", "settings"] });
    },
  });

  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("admin.updateSetting")}</Button>
      {open && (
        <div className="dialog-backdrop">
          <form
            className="dialog auth-form"
            onSubmit={(e) => {
              e.preventDefault();
              mutation.mutate();
            }}
          >
            <h2>{t("admin.updateSetting")}</h2>
            <Field label={t("admin.field.key")} value={key} set={setKey} />
            <Field label={t("admin.field.value")} value={value} set={setValue} />
            <label className="check-line">
              <input type="checkbox" checked={secret} onChange={(e) => setSecret(e.target.checked)} />
              {t("admin.field.is_secret")}
            </label>
            {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
            <div className="button-row">
              <Button type="submit" disabled={mutation.isPending}>
                {t(mutation.isPending ? "common.submitting" : "common.save")}
              </Button>
              <button type="button" className="button button--secondary" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        </div>
      )}
    </>
  );
}

function Field({
  label,
  value,
  set,
  type = "text",
}: {
  label: string;
  value: string;
  set: (v: string) => void;
  type?: string;
}) {
  return (
    <label className="form-field">
      {label}
      <input className="form-input" type={type} required value={value} onChange={(e) => set(e.target.value)} />
    </label>
  );
}

function Shell({
  locale,
  setLocale,
  t,
  actions,
  children,
}: {
  locale: Locale;
  setLocale: (v: Locale) => void;
  t: T;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <AppShell
      title={t("admin.appName")}
      actions={
        <>
          <label>
            <span className="sr-only">{t("common.language")}</span>
            <select
              className="locale-select"
              value={locale}
              onChange={(e) => setLocale(e.target.value as Locale)}
            >
              <option value="zh-CN">{t("common.locale.zh-CN")}</option>
              <option value="en-US">{t("common.locale.en-US")}</option>
            </select>
          </label>
          {actions}
        </>
      }
    >
      {children}
    </AppShell>
  );
}

function AdminTOTP({
  principal,
  csrf,
  t,
}: {
  principal: AdminPrincipal;
  csrf: string;
  t: T;
}) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [setup, setSetup] = useState<TOTPSetup | null>(null);
  const [code, setCode] = useState("");

  const begin = useMutation({
    mutationFn: () =>
      apiRequest<TOTPSetup>("/api/v1/admin/auth/totp/setup", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
      }),
    onSuccess: (value) => {
      setSetup(value);
      setOpen(true);
    },
  });

  const enable = useMutation({
    mutationFn: () =>
      apiRequest<{ two_factor_enabled: boolean }>("/api/v1/admin/auth/totp/enable", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ code }),
      }),
    onSuccess: () => {
      setOpen(false);
      setSetup(null);
      void client.invalidateQueries({ queryKey: ["admin-auth"] });
    },
  });

  if (principal.two_factor_enabled) {
    return <span className="status-badge status-badge--success">{t("admin.twoFactorEnabled")}</span>;
  }

  return (
    <>
      <button
        type="button"
        className="button button--secondary"
        onClick={() => begin.mutate()}
        disabled={begin.isPending}
      >
        {t("admin.enableTwoFactor")}
      </button>
      {(begin.isError || enable.isError) && !open && (
        <Alert tone="danger">{errorText(begin.error ?? enable.error, t)}</Alert>
      )}
      {open && setup && (
        <div className="dialog-backdrop">
          <form
            className="dialog auth-form"
            onSubmit={(event) => {
              event.preventDefault();
              enable.mutate();
            }}
          >
            <h2>{t("admin.twoFactorSetup")}</h2>
            <p>{t("admin.twoFactorInstructions")}</p>
            <label className="form-field">
              {t("admin.twoFactorSecret")}
              <input className="form-input" readOnly value={setup.secret} />
            </label>
            <label className="form-field">
              {t("admin.twoFactorURI")}
              <textarea className="form-input textarea" readOnly value={setup.otpauth_uri} />
            </label>
            <label className="form-field">
              {t("auth.totp")}
              <input
                className="form-input"
                inputMode="numeric"
                autoComplete="one-time-code"
                pattern="[0-9]{6}"
                required
                value={code}
                onChange={(event) => setCode(event.target.value)}
              />
            </label>
            {enable.isError && <Alert tone="danger">{errorText(enable.error, t)}</Alert>}
            <div className="button-row">
              <Button type="submit" disabled={enable.isPending}>
                {t(enable.isPending ? "common.submitting" : "admin.confirmTwoFactor")}
              </Button>
              <button type="button" className="button button--secondary" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        </div>
      )}
    </>
  );
}

function Logout({ csrf, t }: { csrf: string; t: T }) {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<{ logged_out: boolean }>("/api/v1/admin/auth/logout", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
      }),
    onSuccess: () => client.setQueryData(["admin-auth"], undefined),
  });

  return (
    <button
      type="button"
      className="button button--secondary"
      onClick={() => mutation.mutate()}
      disabled={mutation.isPending}
    >
      {t("auth.logout")}
    </button>
  );
}

function useHashPage(): [Page, (v: Page) => void] {
  const read = () => {
    const value = window.location.hash.slice(1) as Page;
    return pages.some((p) => p.key === value) ? value : "dashboard";
  };
  const [page, setState] = useState<Page>(read);
  useEffect(() => {
    const listener = () => setState(read());
    window.addEventListener("hashchange", listener);
    return () => window.removeEventListener("hashchange", listener);
  }, []);
  return [
    page,
    (value) => {
      window.location.hash = value;
      setState(value);
    },
  ];
}

function errorText(error: unknown, t: T) {
  return error instanceof ApiRequestError ? t(error.messageKey as MessageKey) : t("errors.internal");
}

function statusText(status: string, t: T) {
  const key = `status.${status}` as MessageKey;
  return t(key) === key ? status : t(key);
}
