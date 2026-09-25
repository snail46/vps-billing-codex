import {
  ApiRequestError,
  apiRequest,
  type AuthData,
  type BillingProfile,
  type CatalogItem,
  type Instance,
  type InstanceNetwork,
  type Invoice,
  type ItemList,
  type Notification,
  type Operation,
  type OperationAccepted,
  type Order,
  type PortForwardMapping,
  type Subscription,
  type Ticket,
  type TrafficRecord,
  type UsageSummary,
  type UserPrincipal,
  type Wallet,
} from "@vps-billing/api-types";
import { type Locale, type MessageKey, useI18n } from "@vps-billing/i18n";
import { Alert, AppShell, Button, Card, EmptyState, ErrorState, OperationProgress, Skeleton, StatusBadge } from "@vps-billing/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useEffect, useRef, useState } from "react";

type Page = "dashboard" | "products" | "instances" | "subscriptions" | "orders" | "invoices" | "wallet" | "notifications" | "tickets" | "account";
type T = (key: MessageKey) => string;

export function App() {
  const { locale, setLocale, t } = useI18n();
  const queryClient = useQueryClient();
  const auth = useQuery({ queryKey: ["auth"], queryFn: () => apiRequest<AuthData<UserPrincipal>>("/api/v1/auth/me"), retry: false });
  const [page, setPage] = useHashPage();
  const localeInitialized = useRef(false);

  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = t("user.appName");
  }, [locale, t]);

  useEffect(() => {
    if (auth.data?.principal.locale && !localeInitialized.current) {
      localeInitialized.current = true;
      setLocale(auth.data.principal.locale);
    }
  }, [auth.data?.principal.locale, setLocale]);

  if (auth.isPending) {
    return (
      <AppShell title={t("user.appName")} actions={<LanguageSelect locale={locale} setLocale={setLocale} t={t} />}>
        <Skeleton label={t("common.loading")} />
      </AppShell>
    );
  }

  if (auth.isError && (!(auth.error instanceof ApiRequestError) || auth.error.status !== 401)) {
    return (
      <AppShell title={t("user.appName")}>
        <ErrorState title={errorText(auth.error, t)} retry={() => void auth.refetch()} retryLabel={t("common.retry")} />
      </AppShell>
    );
  }

  if (!auth.data) {
    return <AuthScreen locale={locale} setLocale={setLocale} t={t} onAuthenticated={(data) => queryClient.setQueryData(["auth"], data)} />;
  }

  const navItems: Page[] = ["dashboard", "products", "instances", "subscriptions", "orders", "invoices", "wallet", "notifications", "tickets", "account"];

  return (
    <AppShell
      title={t("user.appName")}
      actions={
        <>
          <LanguageSelect locale={locale} setLocale={setLocale} t={t} />
          <span className="mono" style={{ fontSize: "13px", color: "#526078", margin: "0 8px" }}>
            {auth.data.principal.email}
          </span>
          <Logout csrf={auth.data.csrf_token} t={t} />
        </>
      }
    >
      <div className="portal-layout">
        <nav className="portal-nav" aria-label={t("user.appName")}>
          {navItems.map((item) => (
            <button key={item} type="button" data-active={page === item} onClick={() => setPage(item)}>
              {t(`nav.${item}` as MessageKey)}
            </button>
          ))}
        </nav>
        <div className="portal-content">
          <PageView page={page} locale={locale} t={t} csrf={auth.data.csrf_token} principal={auth.data.principal} navigate={setPage} />
        </div>
      </div>
    </AppShell>
  );
}

export function AuthScreen({
  locale,
  setLocale,
  t,
  onAuthenticated,
}: {
  locale: Locale;
  setLocale: (value: Locale) => void;
  t: T;
  onAuthenticated: (data: AuthData<UserPrincipal>) => void;
}) {
  const [register, setRegister] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<AuthData<UserPrincipal>>(`/api/v1/auth/${register ? "register" : "login"}`, {
        method: "POST",
        body: JSON.stringify({ email, password, locale, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone }),
      }),
    onSuccess: onAuthenticated,
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    mutation.mutate();
  };

  return (
    <AppShell title={t("user.appName")} actions={<LanguageSelect locale={locale} setLocale={setLocale} t={t} />}>
      <div className="auth-card">
        <Card>
          <h1>{t(register ? "auth.switchToRegister" : "auth.title")}</h1>
          <form className="auth-form" onSubmit={submit}>
            <label className="form-field">
              {t("auth.email")}
              <input className="form-input" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
            </label>
            <label className="form-field">
              {t("auth.password")}
              <input
                className="form-input"
                type="password"
                autoComplete={register ? "new-password" : "current-password"}
                minLength={12}
                maxLength={128}
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
            {mutation.isError && (
              <p className="form-error" role="alert">
                {errorText(mutation.error, t)}
              </p>
            )}
            <Button type="submit" disabled={mutation.isPending}>
              {t(mutation.isPending ? "auth.pending" : register ? "auth.register" : "auth.login")}
            </Button>
            <button
              className="link-button"
              type="button"
              onClick={() => {
                setRegister(!register);
                mutation.reset();
              }}
            >
              {t(register ? "auth.switchToLogin" : "auth.switchToRegister")}
            </button>
          </form>
        </Card>
      </div>
    </AppShell>
  );
}

function PageView({
  page,
  locale,
  t,
  csrf,
  principal,
  navigate,
}: {
  page: Page;
  locale: Locale;
  t: T;
  csrf: string;
  principal: UserPrincipal;
  navigate: (p: Page) => void;
}) {
  switch (page) {
    case "dashboard":
      return <Dashboard locale={locale} t={t} navigate={navigate} principal={principal} />;
    case "products":
      return <Catalog locale={locale} t={t} csrf={csrf} navigate={navigate} />;
    case "instances":
      return <Instances locale={locale} t={t} csrf={csrf} navigate={navigate} />;
    case "subscriptions":
      return <Subscriptions locale={locale} t={t} csrf={csrf} navigate={navigate} />;
    case "orders":
      return <Orders locale={locale} t={t} navigate={navigate} />;
    case "invoices":
      return <Invoices locale={locale} t={t} />;
    case "wallet":
      return <WalletPage locale={locale} t={t} />;
    case "notifications":
      return <Notifications t={t} csrf={csrf} locale={locale} />;
    case "tickets":
      return <Tickets locale={locale} t={t} csrf={csrf} />;
    case "account":
      return <Account principal={principal} locale={locale} t={t} csrf={csrf} />;
  }
}

function Dashboard({ locale, t, navigate, principal }: { locale: Locale; t: T; navigate: (p: Page) => void; principal: UserPrincipal }) {
  const [now] = useState(() => Date.now());
  const instances = useQuery({ queryKey: ["instances"], queryFn: () => apiRequest<ItemList<Instance>>("/api/v1/instances") });
  const wallet = useQuery({ queryKey: ["wallet"], queryFn: () => apiRequest<Wallet>("/api/v1/wallet?currency=USD") });
  const subscriptions = useQuery({ queryKey: ["subscriptions"], queryFn: () => apiRequest<ItemList<Subscription>>("/api/v1/subscriptions") });
  const notifications = useQuery({ queryKey: ["notifications"], queryFn: () => apiRequest<ItemList<Notification>>("/api/v1/notifications") });
  const orders = useQuery({ queryKey: ["orders"], queryFn: () => apiRequest<ItemList<Order>>("/api/v1/orders") });

  const queries = [instances, wallet, subscriptions, notifications, orders];
  const loaded = queries.some((q) => q.isSuccess);
  const failed = queries.some((q) => q.isError);

  if (!loaded && queries.some((q) => q.isPending)) return <Skeleton label={t("common.loading")} />;
  if (!loaded && failed) {
    return <ErrorState title={t("errors.internal")} retry={() => queries.forEach((q) => void q.refetch())} retryLabel={t("common.retry")} />;
  }

  const serverList = instances.data?.items ?? [];
  const orderList = orders.data?.items ?? [];
  const soon = now + 7 * 86400000;
  const runningCount = serverList.filter((i) => i.observed_state === "running").length;
  const attentionCount = serverList.filter((i) => ["error", "unknown"].includes(i.observed_state)).length;
  const expiringCount = (subscriptions.data?.items ?? []).filter((s) => s.current_period_end && Date.parse(s.current_period_end) < soon).length;
  const unreadCount = (notifications.data?.items ?? []).filter((n) => !n.read_at).length;

  const activeTransitionInstances = serverList.filter((i) => ["provisioning", "restarting", "reinstalling"].includes(i.observed_state));

  return (
    <section>
      <div className="card-heading">
        <div>
          <PageTitle>{t("dashboard.title")}</PageTitle>
          <p style={{ color: "#526078", marginTop: "-16px", marginBottom: "20px" }}>
            {t("dashboard.welcome")}, <strong>{principal.email}</strong>
          </p>
        </div>
      </div>

      {failed && <Alert tone="warning">{t("common.partialError")}</Alert>}

      {activeTransitionInstances.length > 0 && (
        <Alert tone="info">
          <strong>{t("dashboard.activeOperations")}:</strong>{" "}
          {activeTransitionInstances.map((inst) => `${inst.name} (${statusText(inst.observed_state, t)})`).join(", ")}
        </Alert>
      )}

      <div className="quick-actions-bar">
        <button type="button" className="quick-action-btn" onClick={() => navigate("products")}>
          + {t("dashboard.orderVPS")}
        </button>
        <button type="button" className="quick-action-btn" onClick={() => navigate("instances")}>
          {t("dashboard.myServers")}
        </button>
        <button type="button" className="quick-action-btn" onClick={() => navigate("wallet")}>
          {t("dashboard.myWallet")}
        </button>
        <button type="button" className="quick-action-btn" onClick={() => navigate("tickets")}>
          {t("dashboard.newTicket")}
        </button>
      </div>

      <div className="metric-grid">
        <Metric label={t("dashboard.serverCount")} value={String(serverList.length)} onClick={() => navigate("instances")} />
        <Metric label={t("dashboard.runningCount")} value={String(runningCount)} />
        <Metric label={t("dashboard.attentionCount")} value={String(attentionCount)} />
        <Metric
          label={t("dashboard.balance")}
          value={wallet.data ? money(wallet.data.available_balance_minor, wallet.data.currency, locale) : "—"}
          onClick={() => navigate("wallet")}
        />
        <Metric label={t("dashboard.expiring")} value={String(expiringCount)} onClick={() => navigate("subscriptions")} />
        <Metric label={t("dashboard.unread")} value={String(unreadCount)} onClick={() => navigate("notifications")} />
      </div>

      <div className="dashboard-sections">
        <Card>
          <div className="card-heading">
            <h2>{t("dashboard.recentServers")}</h2>
            <button type="button" className="link-button" onClick={() => navigate("instances")}>
              {t("common.view")} →
            </button>
          </div>
          {serverList.length === 0 ? (
            <EmptyState title={t("dashboard.noRecentServers")} action={<Button onClick={() => navigate("products")}>{t("dashboard.orderVPS")}</Button>} />
          ) : (
            <ul className="plain-list">
              {serverList.slice(0, 3).map((item) => (
                <li key={item.id}>
                  <div>
                    <strong>{item.name}</strong>
                    <div style={{ color: "#526078", fontSize: "13px" }}>
                      <span className="mono">{item.primary_ipv4 || item.primary_ipv6 || "—"}</span> · {localized(item.plan_name_i18n, locale)}
                    </div>
                  </div>
                  <StatusBadge status={item.observed_state} label={statusText(item.observed_state, t)} />
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card>
          <div className="card-heading">
            <h2>{t("dashboard.recentOrders")}</h2>
            <button type="button" className="link-button" onClick={() => navigate("orders")}>
              {t("common.view")} →
            </button>
          </div>
          {orderList.length === 0 ? (
            <EmptyState title={t("dashboard.noRecentOrders")} />
          ) : (
            <ul className="plain-list">
              {orderList.slice(0, 3).map((order) => (
                <li key={order.id}>
                  <div>
                    <strong>{order.order_no}</strong>
                    <div style={{ color: "#526078", fontSize: "13px" }}>
                      {t(`orders.kind.${order.kind}` as MessageKey)} · {money(order.total_minor, order.currency, locale)}
                    </div>
                  </div>
                  <StatusBadge status={order.status} label={statusText(order.status, t)} />
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </section>
  );
}

function Catalog({ locale, t, csrf, navigate }: { locale: Locale; t: T; csrf: string; navigate: (p: Page) => void }) {
  const query = useQuery({ queryKey: ["products"], queryFn: () => apiRequest<ItemList<CatalogItem>>("/api/v1/products") });
  const [selected, setSelected] = useState<CatalogItem | null>(null);
  const [quantity, setQuantity] = useState(1);
  const [promotionCode, setPromotionCode] = useState("");
  const [kind, setKind] = useState<"all" | "vps" | "nat_vps">("all");

  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<Order>("/api/v1/orders", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({
          plan_id: selected?.plan_id,
          quantity,
          promotion_code: promotionCode.trim() || undefined,
        }),
      }),
  });

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const items = query.data.items;
  const visible = kind === "all" ? items : items.filter((item) => item.product_type === kind);

  return (
    <section>
      <div className="card-heading">
        <PageTitle>{t("catalog.title")}</PageTitle>
        <label className="form-field compact-field">
          {t("catalog.type")}
          <select className="form-input" value={kind} onChange={(event) => setKind(event.target.value as typeof kind)}>
            <option value="all">{t("catalog.type.all")}</option>
            <option value="vps">{t("catalog.type.vps")}</option>
            <option value="nat_vps">{t("catalog.type.nat_vps")}</option>
          </select>
        </label>
      </div>

      {visible.length === 0 ? (
        <EmptyState title={t("catalog.empty")} />
      ) : (
        <div className="plan-grid">
          {visible.map((item) => (
            <Card key={item.plan_id}>
              <div className="card-heading">
                <div>
                  <h2>{localized(item.plan_name_i18n, locale)}</h2>
                  <p style={{ color: "#526078", fontSize: "13px", margin: "2px 0 0" }}>
                    {localized(item.product_name_i18n, locale)}
                  </p>
                </div>
                {item.featured && <span className="status-badge status-badge--progress">{t("catalog.featured")}</span>}
              </div>

              <div style={{ display: "flex", flexWrap: "wrap", gap: "6px", margin: "10px 0" }}>
                <span className="status-badge">{t(`catalog.type.${item.product_type}` as MessageKey)}</span>
                <span className="status-badge">{item.region || t("catalog.regionFlexible")}</span>
                {item.shared_ipv4 && <span className="status-badge">Shared IPv4</span>}
                {item.port_forward && <span className="status-badge">Port Forwarding</span>}
                {item.traffic_meter && <span className="status-badge">Metered Traffic</span>}
              </div>

              <p className="price">{money(item.price_minor, item.currency, locale)} / {t(`subscriptions.cycle.${item.billing_cycle}` as MessageKey)}</p>
              {item.setup_fee_minor > 0 && (
                <p style={{ color: "#526078", fontSize: "14px", marginTop: "-8px" }}>
                  {t("catalog.setupFee")}: {money(item.setup_fee_minor, item.currency, locale)}
                </p>
              )}

              <dl className="spec-list">
                <dt>{t("catalog.cpu")}</dt>
                <dd>{item.cpu_cores} Cores</dd>
                <dt>{t("catalog.memory")}</dt>
                <dd>{item.memory_mb} MB</dd>
                <dt>{t("catalog.disk")}</dt>
                <dd>{item.disk_gb} GB</dd>
                <dt>{t("catalog.traffic")}</dt>
                <dd>{item.traffic_gb ? `${item.traffic_gb} GB` : t("catalog.unlimited")}</dd>
                <dt>{t("catalog.bandwidth")}</dt>
                <dd>{item.bandwidth_mbps ? `${item.bandwidth_mbps} Mbps` : t("catalog.unlimited")}</dd>
                {item.port_forward && (
                  <>
                    <dt>{t("catalog.natPorts")}</dt>
                    <dd>{item.nat_port_count}</dd>
                  </>
                )}
              </dl>

              {!item.available && <Alert tone="warning">{t("catalog.unavailable")}</Alert>}

              <Button
                disabled={!item.available}
                onClick={() => {
                  setSelected(item);
                  setQuantity(1);
                  setPromotionCode("");
                  mutation.reset();
                }}
              >
                {t("catalog.buy")}
              </Button>
            </Card>
          ))}
        </div>
      )}

      {selected && (
        <div className="dialog-backdrop" role="presentation">
          <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="checkout-title">
            <h2 id="checkout-title">{t("checkout.title")}</h2>
            <div style={{ background: "#f4f7fb", padding: "12px", borderRadius: "8px", marginBottom: "16px" }}>
              <strong>{localized(selected.plan_name_i18n, locale)}</strong>
              <p style={{ margin: "4px 0 0", color: "#526078", fontSize: "13px" }}>
                {selected.cpu_cores} Cores · {selected.memory_mb} MB RAM · {selected.disk_gb} GB Disk
              </p>
            </div>

            <label className="form-field">
              {t("checkout.quantity")}
              <input
                className="form-input"
                type="number"
                min={1}
                max={20}
                value={quantity}
                onChange={(e) => setQuantity(Math.max(1, Math.min(20, Number(e.target.value) || 1)))}
              />
            </label>

            <label className="form-field" style={{ marginTop: "12px" }}>
              {t("checkout.promotionCode")}
              <input
                className="form-input"
                placeholder="PROMO2026"
                value={promotionCode}
                onChange={(e) => setPromotionCode(e.target.value.toUpperCase())}
              />
            </label>

            <div style={{ margin: "16px 0", borderTop: "1px solid #edf0f5", paddingTop: "12px" }}>
              <div style={{ display: "flex", justifyContent: "space-between", color: "#526078", fontSize: "14px" }}>
                <span>{t("checkout.planPrice")}:</span>
                <span>{money(selected.price_minor * quantity, selected.currency, locale)}</span>
              </div>
              {selected.setup_fee_minor > 0 && (
                <div style={{ display: "flex", justifyContent: "space-between", color: "#526078", fontSize: "14px", marginTop: "4px" }}>
                  <span>{t("catalog.setupFee")}:</span>
                  <span>{money(selected.setup_fee_minor * quantity, selected.currency, locale)}</span>
                </div>
              )}
              <div style={{ display: "flex", justifyContent: "space-between", fontWeight: 700, fontSize: "16px", marginTop: "8px" }}>
                <span>{t("checkout.total")}:</span>
                <span>{money((selected.price_minor + selected.setup_fee_minor) * quantity, selected.currency, locale)}</span>
              </div>
            </div>

            {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}

            {mutation.isSuccess && (
              <div style={{ marginBottom: "16px" }}>
                <Alert tone="success">{t("checkout.created")}</Alert>
                <div style={{ background: "#f9fafb", padding: "12px", borderRadius: "8px", fontSize: "13px" }}>
                  <div><strong>{t("orders.number")}:</strong> {mutation.data.order_no}</div>
                  <div><strong>{t("orders.paymentStatus")}:</strong> {statusText(mutation.data.payment_status || "pending", t)}</div>
                </div>
              </div>
            )}

            <div className="button-row">
              {mutation.isSuccess ? (
                <>
                  <Button onClick={() => navigate("orders")}>{t("checkout.viewOrder")}</Button>
                  <button className="button button--secondary" type="button" onClick={() => setSelected(null)}>
                    {t("common.close")}
                  </button>
                </>
              ) : (
                <>
                  <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
                    {t(mutation.isPending ? "common.submitting" : "checkout.create")}
                  </Button>
                  <button className="button button--secondary" type="button" onClick={() => setSelected(null)}>
                    {t("common.cancel")}
                  </button>
                </>
              )}
            </div>
          </section>
        </div>
      )}
    </section>
  );
}

function Instances({
  locale,
  t,
  csrf,
  navigate,
}: {
  locale: Locale;
  t: T;
  csrf: string;
  navigate: (p: Page) => void;
}) {
  const query = useQuery({ queryKey: ["instances"], queryFn: () => apiRequest<ItemList<Instance>>("/api/v1/instances") });
  const [selected, setSelected] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [filterState, setFilterState] = useState<string>("all");

  if (selected) {
    return <InstanceDetail id={selected} locale={locale} t={t} csrf={csrf} back={() => setSelected(null)} />;
  }

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const items = query.data.items;
  const filtered = items.filter((item) => {
    const matchesSearch = item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.primary_ipv4.includes(search) ||
      item.primary_ipv6.includes(search);
    const matchesState = filterState === "all" || item.observed_state === filterState;
    return matchesSearch && matchesState;
  });

  return (
    <section>
      <PageTitle>{t("instances.title")}</PageTitle>

      {items.length === 0 ? (
        <EmptyState title={t("instances.empty")} action={<Button onClick={() => navigate("products")}>{t("nav.products")}</Button>} />
      ) : (
        <>
          <div className="filter-bar">
            <input
              className="form-input"
              style={{ minWidth: "220px" }}
              placeholder={t("admin.search")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <select className="locale-select" value={filterState} onChange={(e) => setFilterState(e.target.value)}>
              <option value="all">{t("common.status")}: {t("catalog.type.all")}</option>
              <option value="running">{statusText("running", t)}</option>
              <option value="stopped">{statusText("stopped", t)}</option>
              <option value="error">{statusText("error", t)}</option>
            </select>
          </div>

          <div className="card-list">
            {filtered.map((item) => (
              <Card key={item.id}>
                <div className="card-heading">
                  <div>
                    <h2>{item.name}</h2>
                    <p style={{ color: "#526078", margin: "2px 0 0" }}>{localized(item.plan_name_i18n, locale)}</p>
                  </div>
                  <StatusBadge status={item.observed_state} label={statusText(item.observed_state, t)} />
                </div>
                <div style={{ display: "flex", alignItems: "center", margin: "8px 0" }}>
                  <span className="mono">{item.primary_ipv4 || item.primary_ipv6 || "—"}</span>
                  {(item.primary_ipv4 || item.primary_ipv6) && (
                    <CopyButton text={item.primary_ipv4 || item.primary_ipv6} t={t} />
                  )}
                </div>
                <p style={{ color: "#526078", fontSize: "13px", margin: "0 0 16px" }}>
                  {item.cpu_cores} CPU · {item.memory_mb} MB RAM · {item.disk_gb} GB Disk
                </p>
                <Button onClick={() => setSelected(item.id)}>{t("common.view")}</Button>
              </Card>
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function InstanceDetail({
  id,
  locale,
  t,
  csrf,
  back,
}: {
  id: string;
  locale: Locale;
  t: T;
  csrf: string;
  back: () => void;
}) {
  const item = useQuery({ queryKey: ["instance", id], queryFn: () => apiRequest<Instance>(`/api/v1/instances/${id}`) });
  const networks = useQuery({ queryKey: ["instance-networks", id], queryFn: () => apiRequest<ItemList<InstanceNetwork>>(`/api/v1/instances/${id}/networks`) });
  const traffic = useQuery({ queryKey: ["instance-traffic", id], queryFn: () => apiRequest<ItemList<TrafficRecord>>(`/api/v1/instances/${id}/traffic`) });
  const usage = useQuery({ queryKey: ["instance-usage", id], queryFn: () => apiRequest<UsageSummary>(`/api/v1/instances/${id}/usage`) });
  const [operationID, setOperationID] = useState<string | null>(null);
  const ports = useQuery({
    queryKey: ["instance-port-forwards", id],
    queryFn: () => apiRequest<ItemList<PortForwardMapping>>(`/api/v1/instances/${id}/port-forwards`),
    refetchInterval: operationID ? 3000 : false,
  });
  const [confirmReinstall, setConfirmReinstall] = useState(false);

  const action = useMutation({
    mutationFn: (name: string) =>
      apiRequest<OperationAccepted>(`/api/v1/instances/${id}/${name}`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() },
        body: name === "reinstall" ? JSON.stringify({ image_id: item.data?.image_id }) : undefined,
      }),
    onSuccess: (data) => setOperationID(data.operation_id),
  });

  if (item.isPending) return <Skeleton label={t("common.loading")} />;
  if (item.isError) {
    return <ErrorState title={errorText(item.error, t)} retry={() => void item.refetch()} retryLabel={t("common.retry")} />;
  }

  const value = item.data;
  const trafficTotal = (traffic.data?.items ?? []).reduce((sum, row) => sum + row.rx_bytes + row.tx_bytes, 0);
  const usedBytes = usage.data?.used_bytes ?? trafficTotal;
  const includedBytes = usage.data?.included_bytes ?? 0;
  const usagePercent = includedBytes > 0 ? Math.min(100, Math.round((usedBytes / includedBytes) * 100)) : 0;

  return (
    <section>
      <button type="button" className="link-button" onClick={back} style={{ marginBottom: "16px" }}>
        ← {t("common.back")}
      </button>

      <div className="card-heading">
        <div>
          <PageTitle>{value.name}</PageTitle>
          <p style={{ color: "#526078", marginTop: "-16px" }}>{localized(value.plan_name_i18n, locale)}</p>
        </div>
        <StatusBadge status={value.observed_state} label={statusText(value.observed_state, t)} />
      </div>

      {(networks.isError || traffic.isError || usage.isError || ports.isError) && (
        <Alert tone="warning">{t("common.partialError")}</Alert>
      )}

      <div className="detail-grid">
        <Card>
          <h2>{t("instance.detail")}</h2>
          <dl className="spec-list">
            <dt>{t("instance.desiredState")}</dt>
            <dd>{statusText(value.desired_state, t)}</dd>
            <dt>{t("instance.observedState")}</dt>
            <dd>{statusText(value.observed_state, t)}</dd>
            <dt>{t("catalog.cpu")}</dt>
            <dd>{value.cpu_cores} Cores</dd>
            <dt>{t("catalog.memory")}</dt>
            <dd>{value.memory_mb} MB</dd>
            <dt>{t("catalog.disk")}</dt>
            <dd>{value.disk_gb} GB</dd>
            <dt>{t("instance.image")}</dt>
            <dd>{value.image_id ?? "—"}</dd>
            <dt>{t("instance.expires")}</dt>
            <dd>{dateTime(value.current_period_end, locale)}</dd>
            <dt>{t("instance.lastSync")}</dt>
            <dd>{dateTime(value.last_synced_at, locale)}</dd>
          </dl>
        </Card>

        <Card>
          <h2>{t("instance.network")}</h2>
          {networks.isPending ? (
            <Skeleton label={t("common.loading")} />
          ) : networks.data?.items.length ? (
            <ul className="plain-list">
              {networks.data.items.map((network) => (
                <li key={network.id} style={{ alignItems: "center" }}>
                  <span>
                    <span className="status-badge" style={{ marginRight: "8px" }}>{network.type}</span>
                    <code className="mono">{network.address}{network.prefix !== null ? `/${network.prefix}` : ""}</code>
                  </span>
                  <CopyButton text={network.address} t={t} />
                </li>
              ))}
            </ul>
          ) : (
            <EmptyState title={t("instance.noNetwork")} />
          )}
        </Card>

        <Card>
          <h2>{t("instance.traffic")}</h2>
          <p className="traffic-total">{formatBytes(usedBytes, locale)}</p>
          {includedBytes > 0 && (
            <div style={{ marginBottom: "16px" }}>
              <div style={{ display: "flex", justifyContent: "space-between", fontSize: "13px", color: "#526078" }}>
                <span>{t("instance.usageProgress")}</span>
                <span>{usagePercent}% ({formatBytes(usedBytes, locale)} / {formatBytes(includedBytes, locale)})</span>
              </div>
              <div className="progress-bar-wrap">
                <div
                  className={`progress-bar-fill ${usagePercent > 90 ? "progress-bar-fill--danger" : usagePercent > 75 ? "progress-bar-fill--warning" : ""}`}
                  style={{ width: `${usagePercent}%` }}
                />
              </div>
            </div>
          )}

          {usage.data && (
            <dl className="spec-list">
              <dt>{t("instance.includedTraffic")}</dt>
              <dd>{formatBytes(usage.data.included_bytes, locale)}</dd>
              <dt>{t("instance.overage")}</dt>
              <dd>{formatBytes(usage.data.overage_bytes, locale)}</dd>
              <dt>{t("instance.estimatedCharge")}</dt>
              <dd>{money(usage.data.estimated_minor, usage.data.currency, locale)}</dd>
            </dl>
          )}

          {traffic.isPending ? (
            <Skeleton label={t("common.loading")} />
          ) : traffic.data?.items.length ? (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{t("common.date")}</th>
                    <th>{t("instance.rx")}</th>
                    <th>{t("instance.tx")}</th>
                  </tr>
                </thead>
                <tbody>
                  {traffic.data.items.slice(0, 5).map((row) => (
                    <tr key={`${row.period_start}-${row.source}`}>
                      <td>{dateTime(row.period_start, locale)}</td>
                      <td>{formatBytes(row.rx_bytes, locale)}</td>
                      <td>{formatBytes(row.tx_bytes, locale)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <EmptyState title={t("instance.noTraffic")} />
          )}
        </Card>

        <PortForwards
          instanceID={id}
          items={ports.data?.items ?? []}
          pending={ports.isPending}
          csrf={csrf}
          t={t}
          accepted={setOperationID}
        />
      </div>

      <div style={{ marginTop: "24px" }}>
        <Card>
          <h2>{t("common.actions")}</h2>
          {action.isError && <Alert tone="danger">{errorText(action.error, t)}</Alert>}
          <div className="button-row">
            <Button
              disabled={action.isPending || value.observed_state === "running" || !!operationID}
              onClick={() => action.mutate("start")}
            >
              {t("instance.actions.start")}
            </Button>
            <Button
              disabled={action.isPending || value.observed_state === "stopped" || !!operationID}
              onClick={() => action.mutate("stop")}
            >
              {t("instance.actions.stop")}
            </Button>
            <Button
              disabled={action.isPending || value.observed_state !== "running" || !!operationID}
              onClick={() => action.mutate("restart")}
            >
              {t("instance.actions.restart")}
            </Button>
            <button
              className="button button--danger"
              disabled={action.isPending || !!operationID}
              type="button"
              onClick={() => setConfirmReinstall(true)}
            >
              {t("instance.actions.reinstall")}
            </button>
          </div>
        </Card>
      </div>

      {operationID && (
        <div style={{ marginTop: "24px" }}>
          <Card>
            <div className="card-heading">
              <h2>{t("instance.activity")}</h2>
              <button className="link-button" type="button" onClick={() => setOperationID(null)}>
                {t("common.close")}
              </button>
            </div>
            <Alert tone="info">{t("instance.actionAccepted")}</Alert>
            <OperationView id={operationID} t={t} onCompleted={() => { void item.refetch(); void ports.refetch(); }} />
          </Card>
        </div>
      )}

      {confirmReinstall && (
        <div className="dialog-backdrop">
          <section className="dialog" role="alertdialog" aria-modal="true">
            <h2>{t("instance.actions.reinstall")}</h2>
            <Alert tone="danger">{t("instance.actions.reinstallWarning")}</Alert>
            <div className="button-row">
              <button
                className="button button--danger"
                type="button"
                onClick={() => {
                  setConfirmReinstall(false);
                  action.mutate("reinstall");
                }}
              >
                {t("instance.actions.confirmReinstall")}
              </button>
              <button className="button button--secondary" type="button" onClick={() => setConfirmReinstall(false)}>
                {t("common.cancel")}
              </button>
            </div>
          </section>
        </div>
      )}
    </section>
  );
}

function PortForwards({
  instanceID,
  items,
  pending,
  csrf,
  t,
  accepted,
}: {
  instanceID: string;
  items: PortForwardMapping[];
  pending: boolean;
  csrf: string;
  t: T;
  accepted: (id: string) => void;
}) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [protocol, setProtocol] = useState<"tcp" | "udp">("tcp");
  const [publicPort, setPublicPort] = useState("");
  const [guestPort, setGuestPort] = useState("22");
  const [description, setDescription] = useState("");

  const add = useMutation({
    mutationFn: () =>
      apiRequest<OperationAccepted>(`/api/v1/instances/${instanceID}/port-forwards`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({ protocol, public_port: Number(publicPort), guest_port: Number(guestPort), description }),
      }),
    onSuccess: (value) => {
      accepted(value.operation_id);
      setOpen(false);
      setPublicPort("");
      setDescription("");
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) =>
      apiRequest<OperationAccepted>(`/api/v1/instances/${instanceID}/port-forwards/${id}`, {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() },
      }),
    onSuccess: (value) => {
      accepted(value.operation_id);
      void client.invalidateQueries({ queryKey: ["instance-port-forwards", instanceID] });
    },
  });

  return (
    <Card>
      <div className="card-heading">
        <h2>{t("instance.portForwards")}</h2>
        <Button onClick={() => setOpen(true)}>{t("instance.portForward.add")}</Button>
      </div>

      {pending ? (
        <Skeleton label={t("common.loading")} />
      ) : items.length ? (
        <ul className="plain-list">
          {items.map((mapping) => (
            <li key={mapping.id} style={{ alignItems: "center" }}>
              <div>
                <StatusBadge status={mapping.status} label={statusText(mapping.status, t)} />{" "}
                <span className="mono" style={{ marginLeft: "8px", fontWeight: 600 }}>
                  {mapping.protocol.toUpperCase()} {mapping.public_ip}:{mapping.public_port} → {mapping.guest_port}
                </span>
                {mapping.description && <span style={{ color: "#526078", marginLeft: "8px", fontSize: "13px" }}>({mapping.description})</span>}
              </div>
              <button
                type="button"
                className="link-button"
                style={{ color: "#b42318" }}
                disabled={remove.isPending}
                onClick={() => remove.mutate(mapping.id)}
              >
                {t("common.delete")}
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <EmptyState title={t("instance.portForward.empty")} />
      )}

      {(add.isError || remove.isError) && <Alert tone="danger">{errorText(add.error ?? remove.error, t)}</Alert>}

      {open && (
        <div className="dialog-backdrop">
          <form
            className="dialog auth-form"
            onSubmit={(event) => {
              event.preventDefault();
              add.mutate();
            }}
          >
            <h2>{t("instance.portForward.add")}</h2>
            <label className="form-field">
              {t("instance.portForward.protocol")}
              <select className="form-input" value={protocol} onChange={(e) => setProtocol(e.target.value as typeof protocol)}>
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </label>
            <label className="form-field">
              {t("instance.portForward.publicPort")}
              <input
                className="form-input"
                type="number"
                min="1"
                max="65535"
                required
                placeholder="10000-65535"
                value={publicPort}
                onChange={(e) => setPublicPort(e.target.value)}
              />
            </label>
            <label className="form-field">
              {t("instance.portForward.guestPort")}
              <input
                className="form-input"
                type="number"
                min="1"
                max="65535"
                required
                value={guestPort}
                onChange={(e) => setGuestPort(e.target.value)}
              />
            </label>
            <label className="form-field">
              {t("instance.portForward.description")}
              <input className="form-input" maxLength={255} value={description} onChange={(e) => setDescription(e.target.value)} />
            </label>

            {add.isError && <Alert tone="danger">{errorText(add.error, t)}</Alert>}

            <div className="button-row">
              <Button type="submit" disabled={add.isPending}>
                {t(add.isPending ? "common.submitting" : "common.save")}
              </Button>
              <button className="button button--secondary" type="button" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        </div>
      )}
    </Card>
  );
}

function OperationView({ id, t, onCompleted }: { id: string; t: T; onCompleted?: () => void }) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["operation", id],
    queryFn: () => apiRequest<Operation>(`/api/v1/operations/${id}`),
    refetchInterval: (q) => (isTerminal(q.state.data?.status) ? false : 2000),
  });

  useEffect(() => {
    if (isTerminal(query.data?.status)) {
      onCompleted?.();
    }
  }, [query.data?.status, onCompleted]);

  useEffect(() => {
    const stream = new EventSource("/api/v1/events", { withCredentials: true });
    stream.onmessage = () => {
      void client.invalidateQueries({ queryKey: ["operation", id] });
    };
    return () => stream.close();
  }, [client, id]);

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  return <OperationProgress operation={query.data} translate={(key) => t(key as MessageKey)} />;
}

function Subscriptions({
  locale,
  t,
  csrf,
  navigate,
}: {
  locale: Locale;
  t: T;
  csrf: string;
  navigate: (p: Page) => void;
}) {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["subscriptions"], queryFn: () => apiRequest<ItemList<Subscription>>("/api/v1/subscriptions") });

  const renew = useMutation({
    mutationFn: (id: string) =>
      apiRequest<Order>(`/api/v1/subscriptions/${id}/renewals`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() },
        body: "{}",
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["orders"] });
      void client.invalidateQueries({ queryKey: ["invoices"] });
    },
  });

  const cancel = useMutation({
    mutationFn: ({ id, value }: { id: string; value: boolean }) =>
      apiRequest<Subscription>(`/api/v1/subscriptions/${id}/cancel-at-period-end`, {
        method: "PUT",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ cancel: value }),
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["subscriptions"] }),
  });

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  return (
    <section>
      <PageTitle>{t("subscriptions.title")}</PageTitle>

      {query.data.items.length === 0 ? (
        <EmptyState title={t("subscriptions.empty")} action={<Button onClick={() => navigate("products")}>{t("dashboard.orderVPS")}</Button>} />
      ) : (
        <div className="card-list">
          {query.data.items.map((item) => (
            <Card key={item.id}>
              <div className="card-heading">
                <div>
                  <h2>{localized(item.plan_name_i18n, locale)}</h2>
                  <p style={{ color: "#526078", margin: "2px 0 0" }}>
                    {money(item.price_minor, item.currency, locale)} · {t(`subscriptions.cycle.${item.billing_cycle}` as MessageKey)}
                  </p>
                </div>
                <StatusBadge status={item.status} label={statusText(item.status, t)} />
              </div>

              <dl className="spec-list">
                <dt>{t("subscriptions.periodEnd")}</dt>
                <dd>{dateTime(item.current_period_end, locale)}</dd>
                <dt>{t("subscriptions.nextDue")}</dt>
                <dd>{dateTime(item.next_due_at, locale)}</dd>
                <dt>{t("subscriptions.renewal")}</dt>
                <dd>{item.cancel_at_period_end ? t("subscriptions.cancelling") : t("subscriptions.automatic")}</dd>
              </dl>

              {(renew.isError || cancel.isError) && (
                <Alert tone="danger">{errorText(renew.error ?? cancel.error, t)}</Alert>
              )}

              {renew.isSuccess && (
                <div style={{ marginBottom: "16px" }}>
                  <Alert tone="success">{t("subscriptions.renewalCreated")}</Alert>
                  <Button onClick={() => navigate("orders")}>{t("checkout.viewOrder")}</Button>
                </div>
              )}

              <div className="button-row">
                <Button
                  disabled={renew.isPending || !["active", "past_due", "suspended"].includes(item.status)}
                  onClick={() => renew.mutate(item.id)}
                >
                  {t(renew.isPending ? "common.submitting" : "subscriptions.renew")}
                </Button>
                {["active", "past_due"].includes(item.status) && (
                  <button
                    className="button button--secondary"
                    type="button"
                    disabled={cancel.isPending}
                    onClick={() => cancel.mutate({ id: item.id, value: !item.cancel_at_period_end })}
                  >
                    {t(item.cancel_at_period_end ? "subscriptions.keep" : "subscriptions.cancel")}
                  </button>
                )}
              </div>
            </Card>
          ))}
        </div>
      )}
    </section>
  );
}

function Orders({ locale, t, navigate }: { locale: Locale; t: T; navigate: (p: Page) => void }) {
  const query = useQuery({ queryKey: ["orders"], queryFn: () => apiRequest<ItemList<Order>>("/api/v1/orders") });
  const [selectedOrder, setSelectedOrder] = useState<Order | null>(null);
  const [statusFilter, setStatusFilter] = useState("all");

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const items = query.data.items;
  const filtered = statusFilter === "all" ? items : items.filter((o) => o.status === statusFilter);

  return (
    <section>
      <PageTitle>{t("orders.title")}</PageTitle>

      {items.length === 0 ? (
        <EmptyState title={t("orders.empty")} action={<Button onClick={() => navigate("products")}>{t("dashboard.orderVPS")}</Button>} />
      ) : (
        <>
          <div className="filter-bar">
            <select className="locale-select" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
              <option value="all">{t("common.status")}: {t("catalog.type.all")}</option>
              <option value="pending">{statusText("pending", t)}</option>
              <option value="paid">{statusText("paid", t)}</option>
              <option value="fulfilled">{statusText("fulfilled", t)}</option>
              <option value="cancelled">{statusText("cancelled", t)}</option>
            </select>
          </div>

          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("orders.number")}</th>
                  <th>{t("common.status")}</th>
                  <th>{t("orders.paymentStatus")}</th>
                  <th>{t("orders.kind")}</th>
                  <th>{t("common.amount")}</th>
                  <th>{t("common.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((item) => (
                  <tr key={item.id}>
                    <td className="mono">{item.order_no}</td>
                    <td><StatusBadge status={item.status} label={statusText(item.status, t)} /></td>
                    <td><StatusBadge status={item.payment_status || "pending"} label={statusText(item.payment_status || "pending", t)} /></td>
                    <td>{t(`orders.kind.${item.kind}` as MessageKey)}</td>
                    <td>{money(item.total_minor, item.currency, locale)}</td>
                    <td>
                      <button className="link-button" type="button" onClick={() => setSelectedOrder(item)}>
                        {t("orders.view")}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {selectedOrder && (
        <div className="dialog-backdrop" role="presentation">
          <section className="dialog" role="dialog" aria-modal="true">
            <div className="card-heading">
              <h2>{t("orders.detail")}</h2>
              <button className="button button--secondary" type="button" onClick={() => setSelectedOrder(null)}>
                {t("common.close")}
              </button>
            </div>

            <table className="detail-table">
              <tbody>
                <tr>
                  <th>{t("orders.number")}</th>
                  <td className="mono">{selectedOrder.order_no}</td>
                </tr>
                <tr>
                  <th>{t("common.status")}</th>
                  <td><StatusBadge status={selectedOrder.status} label={statusText(selectedOrder.status, t)} /></td>
                </tr>
                <tr>
                  <th>{t("orders.paymentStatus")}</th>
                  <td><StatusBadge status={selectedOrder.payment_status || "pending"} label={statusText(selectedOrder.payment_status || "pending", t)} /></td>
                </tr>
                <tr>
                  <th>{t("orders.kind")}</th>
                  <td>{t(`orders.kind.${selectedOrder.kind}` as MessageKey)}</td>
                </tr>
                <tr>
                  <th>{t("common.amount")}</th>
                  <td><strong>{money(selectedOrder.total_minor, selectedOrder.currency, locale)}</strong></td>
                </tr>
                {selectedOrder.subscription_id && (
                  <tr>
                    <th>{t("orders.subscription")}</th>
                    <td>
                      <button
                        className="link-button"
                        type="button"
                        onClick={() => {
                          setSelectedOrder(null);
                          navigate("subscriptions");
                        }}
                      >
                        {t("orders.viewSubscription")} →
                      </button>
                    </td>
                  </tr>
                )}
              </tbody>
            </table>

            {(!selectedOrder.payment_status || selectedOrder.payment_status === "pending") && (
              <Alert tone="info">{t("orders.paymentNotice")}</Alert>
            )}

            <div className="button-row" style={{ marginTop: "16px" }}>
              <button className="button button--secondary" type="button" onClick={() => setSelectedOrder(null)}>
                {t("common.close")}
              </button>
            </div>
          </section>
        </div>
      )}
    </section>
  );
}

function Invoices({ locale, t }: { locale: Locale; t: T }) {
  const query = useQuery({ queryKey: ["invoices"], queryFn: () => apiRequest<ItemList<Invoice>>("/api/v1/invoices") });
  const [selectedInvoice, setSelectedInvoice] = useState<Invoice | null>(null);

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const items = query.data.items;

  return (
    <section>
      <PageTitle>{t("invoices.title")}</PageTitle>

      {items.length === 0 ? (
        <EmptyState title={t("invoices.empty")} />
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("invoices.number")}</th>
                <th>{t("common.status")}</th>
                <th>{t("invoices.due")}</th>
                <th>{t("common.amount")}</th>
                <th>{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td className="mono">{item.invoice_no}</td>
                  <td><StatusBadge status={item.status} label={statusText(item.status, t)} /></td>
                  <td>{dateTime(item.due_at, locale)}</td>
                  <td>{money(item.amount_minor, item.currency, locale)}</td>
                  <td>
                    <button className="link-button" type="button" onClick={() => setSelectedInvoice(item)}>
                      {t("common.view")}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {selectedInvoice && (
        <div className="dialog-backdrop" role="presentation">
          <section className="dialog" role="dialog" aria-modal="true">
            <div className="card-heading">
              <h2>{t("invoices.detail")}</h2>
              <button className="button button--secondary" type="button" onClick={() => setSelectedInvoice(null)}>
                {t("common.close")}
              </button>
            </div>

            <table className="detail-table">
              <tbody>
                <tr>
                  <th>{t("invoices.number")}</th>
                  <td className="mono">{selectedInvoice.invoice_no}</td>
                </tr>
                <tr>
                  <th>{t("common.status")}</th>
                  <td><StatusBadge status={selectedInvoice.status} label={statusText(selectedInvoice.status, t)} /></td>
                </tr>
                <tr>
                  <th>{t("invoices.due")}</th>
                  <td>{dateTime(selectedInvoice.due_at, locale)}</td>
                </tr>
                <tr>
                  <th>{t("common.amount")}</th>
                  <td><strong>{money(selectedInvoice.amount_minor, selectedInvoice.currency, locale)}</strong></td>
                </tr>
                {selectedInvoice.order_id && (
                  <tr>
                    <th>{t("invoices.associatedOrder")}</th>
                    <td className="mono">{selectedInvoice.order_id}</td>
                  </tr>
                )}
                {selectedInvoice.subscription_id && (
                  <tr>
                    <th>{t("invoices.associatedSubscription")}</th>
                    <td className="mono">{selectedInvoice.subscription_id}</td>
                  </tr>
                )}
              </tbody>
            </table>

            <div className="button-row">
              <button className="button button--secondary" type="button" onClick={() => setSelectedInvoice(null)}>
                {t("common.close")}
              </button>
            </div>
          </section>
        </div>
      )}
    </section>
  );
}

function WalletPage({ locale, t }: { locale: Locale; t: T }) {
  const query = useQuery({ queryKey: ["wallet"], queryFn: () => apiRequest<Wallet>("/api/v1/wallet?currency=USD") });

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  return (
    <section>
      <PageTitle>{t("wallet.title")}</PageTitle>
      <Card>
        <p style={{ color: "#526078", fontSize: "14px", margin: "0 0 8px" }}>{t("wallet.available")}</p>
        <p className="balance">{money(query.data.available_balance_minor, query.data.currency, locale)}</p>
        <p style={{ color: "#526078", fontSize: "13px", marginTop: "16px" }}>
          Wallet balances are backed by the double-entry accounting ledger. Available funds can be applied to renewals or credited via administrator adjustments.
        </p>
      </Card>
    </section>
  );
}

function Notifications({ t, csrf, locale }: { t: T; csrf: string; locale: Locale }) {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["notifications"], queryFn: () => apiRequest<ItemList<Notification>>("/api/v1/notifications") });
  const [filter, setFilter] = useState<"all" | "unread">("all");

  const mark = useMutation({
    mutationFn: (id: string) =>
      apiRequest<{ read: boolean }>(`/api/v1/notifications/${id}/read`, {
        method: "PUT",
        headers: { "X-CSRF-Token": csrf },
        body: "{}",
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["notifications"] }),
  });

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const items = query.data.items;
  const filtered = filter === "all" ? items : items.filter((n) => !n.read_at);

  return (
    <section>
      <div className="card-heading">
        <PageTitle>{t("notifications.title")}</PageTitle>
        <div className="button-row">
          <button
            className={`button ${filter === "all" ? "" : "button--secondary"}`}
            type="button"
            onClick={() => setFilter("all")}
          >
            {t("catalog.type.all")} ({items.length})
          </button>
          <button
            className={`button ${filter === "unread" ? "" : "button--secondary"}`}
            type="button"
            onClick={() => setFilter("unread")}
          >
            {t("dashboard.unread")} ({items.filter((n) => !n.read_at).length})
          </button>
        </div>
      </div>

      {filtered.length === 0 ? (
        <EmptyState title={t("notifications.empty")} />
      ) : (
        <div className="card-list">
          {filtered.map((item) => (
            <Card key={item.id}>
              <div className="card-heading">
                <div>
                  <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                    <h2>{safeTranslate(item.title_key, t)}</h2>
                    {!item.read_at && <span className="status-badge status-badge--progress">New</span>}
                  </div>
                  <p style={{ margin: "4px 0 8px" }}>{safeTranslate(item.message_key, t)}</p>
                  <small style={{ color: "#667085" }}>{dateTime(item.created_at, locale)}</small>
                </div>
                {!item.read_at && (
                  <Button disabled={mark.isPending} onClick={() => mark.mutate(item.id)}>
                    {t("notifications.markRead")}
                  </Button>
                )}
              </div>
            </Card>
          ))}
        </div>
      )}
    </section>
  );
}

function Tickets({ locale, t, csrf }: { locale: Locale; t: T; csrf: string }) {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["tickets"], queryFn: () => apiRequest<ItemList<Ticket>>("/api/v1/tickets") });
  const [show, setShow] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [subject, setSubject] = useState("");
  const [priority, setPriority] = useState("normal");
  const [message, setMessage] = useState("");

  const create = useMutation({
    mutationFn: () =>
      apiRequest<Ticket>("/api/v1/tickets", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ subject, priority, message }),
      }),
    onSuccess: () => {
      setShow(false);
      setSubject("");
      setMessage("");
      void client.invalidateQueries({ queryKey: ["tickets"] });
    },
  });

  if (selected) {
    return <TicketDetail id={selected} locale={locale} t={t} csrf={csrf} back={() => setSelected(null)} />;
  }

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  return (
    <section>
      <div className="card-heading">
        <PageTitle>{t("tickets.title")}</PageTitle>
        <Button onClick={() => setShow(true)}>{t("tickets.new")}</Button>
      </div>

      {query.data.items.length === 0 ? (
        <EmptyState title={t("tickets.empty")} action={<Button onClick={() => setShow(true)}>{t("tickets.new")}</Button>} />
      ) : (
        <div className="card-list">
          {query.data.items.map((item) => (
            <Card key={item.id}>
              <div className="card-heading">
                <div>
                  <h2>{item.subject}</h2>
                  <p style={{ color: "#526078", fontSize: "13px", margin: "2px 0 0" }}>
                    <span className="mono">{item.ticket_no}</span> · {dateTime(item.updated_at, locale)} ·{" "}
                    <span className="status-badge">{t(`tickets.priority.${item.priority}` as MessageKey)}</span>
                  </p>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                  <StatusBadge status={item.status} label={statusText(item.status, t)} />
                  <Button onClick={() => setSelected(item.id)}>{t("common.view")}</Button>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}

      {show && (
        <div className="dialog-backdrop">
          <form
            className="dialog auth-form"
            onSubmit={(e) => {
              e.preventDefault();
              create.mutate();
            }}
          >
            <h2>{t("tickets.new")}</h2>
            <label className="form-field">
              {t("tickets.subject")}
              <input
                className="form-input"
                minLength={3}
                maxLength={255}
                required
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
              />
            </label>
            <label className="form-field">
              {t("tickets.priority")}
              <select className="form-input" value={priority} onChange={(e) => setPriority(e.target.value)}>
                <option value="low">{t("tickets.priority.low")}</option>
                <option value="normal">{t("tickets.priority.normal")}</option>
                <option value="high">{t("tickets.priority.high")}</option>
              </select>
            </label>
            <label className="form-field">
              {t("tickets.message")}
              <textarea
                className="form-input textarea"
                required
                maxLength={10000}
                value={message}
                onChange={(e) => setMessage(e.target.value)}
              />
            </label>

            {create.isError && <Alert tone="danger">{errorText(create.error, t)}</Alert>}

            <div className="button-row">
              <Button type="submit" disabled={create.isPending}>
                {t(create.isPending ? "common.submitting" : "common.submit")}
              </Button>
              <button className="button button--secondary" type="button" onClick={() => setShow(false)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        </div>
      )}
    </section>
  );
}

function TicketDetail({
  id,
  locale,
  t,
  csrf,
  back,
}: {
  id: string;
  locale: Locale;
  t: T;
  csrf: string;
  back: () => void;
}) {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["ticket", id], queryFn: () => apiRequest<Ticket>(`/api/v1/tickets/${id}`) });
  const [message, setMessage] = useState("");

  const reply = useMutation({
    mutationFn: () =>
      apiRequest(`/api/v1/tickets/${id}/messages`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({ message }),
      }),
    onSuccess: () => {
      setMessage("");
      void client.invalidateQueries({ queryKey: ["ticket", id] });
    },
  });

  if (query.isPending) return <Skeleton label={t("common.loading")} />;
  if (query.isError) {
    return <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />;
  }

  const ticket = query.data;
  const isClosed = ticket.status === "closed" || ticket.status === "resolved";

  return (
    <section>
      <button className="link-button" type="button" onClick={back} style={{ marginBottom: "16px" }}>
        ← {t("common.back")}
      </button>

      <div className="card-heading">
        <div>
          <PageTitle>{ticket.subject}</PageTitle>
          <p style={{ color: "#526078", marginTop: "-16px" }}>
            <span className="mono">{ticket.ticket_no}</span> · {dateTime(ticket.updated_at, locale)} ·{" "}
            <span className="status-badge">{t(`tickets.priority.${ticket.priority}` as MessageKey)}</span>
          </p>
        </div>
        <StatusBadge status={ticket.status} label={statusText(ticket.status, t)} />
      </div>

      <Card>
        <h2>{t("tickets.conversation")}</h2>
        {ticket.messages?.length ? (
          <div className="message-list">
            {ticket.messages.map((item) => (
              <article
                key={item.id}
                style={{
                  borderLeft: item.sender_type === "admin" ? "4px solid #155eef" : "4px solid #667085",
                  background: item.sender_type === "admin" ? "#f0f5ff" : "#f7f9fc",
                }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "6px" }}>
                  <strong>{t(`tickets.sender.${item.sender_type}` as MessageKey)}</strong>
                  <time>{dateTime(item.created_at, locale)}</time>
                </div>
                <p style={{ margin: 0 }}>{item.message}</p>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState title={t("tickets.noMessages")} />
        )}

        {isClosed ? (
          <Alert tone="warning">{t("tickets.closedNotice")}</Alert>
        ) : (
          <form
            className="auth-form"
            onSubmit={(e) => {
              e.preventDefault();
              reply.mutate();
            }}
          >
            <label className="form-field">
              {t("tickets.reply")}
              <textarea
                className="form-input textarea"
                required
                maxLength={10000}
                value={message}
                onChange={(e) => setMessage(e.target.value)}
              />
            </label>
            {reply.isError && <Alert tone="danger">{errorText(reply.error, t)}</Alert>}
            <Button type="submit" disabled={reply.isPending || !message.trim()}>
              {t(reply.isPending ? "common.submitting" : "common.submit")}
            </Button>
          </form>
        )}
      </Card>
    </section>
  );
}

function Account({
  principal,
  locale,
  t,
  csrf,
}: {
  principal: UserPrincipal;
  locale: Locale;
  t: T;
  csrf: string;
}) {
  const query = useQuery({ queryKey: ["billing-profile"], queryFn: () => apiRequest<BillingProfile>("/api/v1/billing-profile") });

  return (
    <section>
      <PageTitle>{t("account.title")}</PageTitle>

      <Card>
        <dl className="spec-list">
          <dt>{t("account.email")}</dt>
          <dd>{principal.email}</dd>
          <dt>{t("account.locale")}</dt>
          <dd>{locale}</dd>
          <dt>{t("account.timezone")}</dt>
          <dd>{principal.timezone}</dd>
        </dl>
        <Alert tone="info">{t("account.session")}</Alert>
      </Card>

      <div style={{ marginTop: "24px" }}>
        <Card>
          <h2>{t("account.billingProfile")}</h2>
          {query.isPending ? (
            <Skeleton label={t("common.loading")} />
          ) : query.isError ? (
            <ErrorState title={errorText(query.error, t)} retry={() => void query.refetch()} retryLabel={t("common.retry")} />
          ) : (
            <BillingProfileForm initial={query.data} csrf={csrf} t={t} />
          )}
        </Card>
      </div>
    </section>
  );
}

function BillingProfileForm({ initial, csrf, t }: { initial: BillingProfile; csrf: string; t: T }) {
  const client = useQueryClient();
  const [legalName, setLegalName] = useState(initial.legal_name || "");
  const [taxID, setTaxID] = useState(initial.tax_id || "");
  const [country, setCountry] = useState(initial.country_code || "");

  const initialAddress = (initial.address || {}) as Record<string, string>;
  const [street, setStreet] = useState(initialAddress.street || "");
  const [city, setCity] = useState(initialAddress.city || "");
  const [postalCode, setPostalCode] = useState(initialAddress.postal_code || "");

  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<BillingProfile>("/api/v1/billing-profile", {
        method: "PUT",
        headers: { "X-CSRF-Token": csrf },
        body: JSON.stringify({
          legal_name: legalName,
          tax_id: taxID,
          country_code: country,
          address: { street, city, postal_code: postalCode },
        }),
      }),
    onSuccess: (value) => client.setQueryData(["billing-profile"], value),
  });

  return (
    <form
      className="auth-form"
      onSubmit={(event) => {
        event.preventDefault();
        mutation.mutate();
      }}
    >
      <AccountField label={t("account.legalName")} value={legalName} set={setLegalName} />
      <AccountField label={t("account.taxID")} value={taxID} set={setTaxID} />
      <AccountField label={t("account.countryCode")} value={country} set={(v) => setCountry(v.toUpperCase().slice(0, 2))} />

      <h3 style={{ margin: "12px 0 4px", fontSize: "16px" }}>{t("account.address")}</h3>
      <AccountField label={t("account.street")} value={street} set={setStreet} />
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
        <AccountField label={t("account.city")} value={city} set={setCity} />
        <AccountField label={t("account.postalCode")} value={postalCode} set={setPostalCode} />
      </div>

      {mutation.isError && <Alert tone="danger">{errorText(mutation.error, t)}</Alert>}
      {mutation.isSuccess && <Alert tone="success">{t("account.saved")}</Alert>}

      <Button type="submit" disabled={mutation.isPending}>
        {t(mutation.isPending ? "common.submitting" : "common.save")}
      </Button>
    </form>
  );
}

function AccountField({ label, value, set }: { label: string; value: string; set: (value: string) => void }) {
  return (
    <label className="form-field">
      {label}
      <input className="form-input" value={value} onChange={(event) => set(event.target.value)} />
    </label>
  );
}

function CopyButton({ text, t }: { text: string; t: T }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    void navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button type="button" className="copy-btn" data-copied={copied} onClick={handleCopy}>
      {copied ? t("common.copied") : t("common.copy")}
    </button>
  );
}

function Logout({ csrf, t }: { csrf: string; t: T }) {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: () =>
      apiRequest<{ logged_out: boolean }>("/api/v1/auth/logout", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
        body: "{}",
      }),
    onSuccess: () => {
      client.clear();
      location.hash = "dashboard";
    },
  });

  return (
    <button type="button" className="button button--secondary" disabled={mutation.isPending} onClick={() => mutation.mutate()}>
      {t("auth.logout")}
    </button>
  );
}

function LanguageSelect({ locale, setLocale, t }: { locale: Locale; setLocale: (locale: Locale) => void; t: T }) {
  return (
    <label>
      <span className="sr-only">{t("common.language")}</span>
      <select className="locale-select" value={locale} onChange={(e) => setLocale(e.target.value as Locale)}>
        <option value="zh-CN">{t("common.locale.zh-CN")}</option>
        <option value="en-US">{t("common.locale.en-US")}</option>
      </select>
    </label>
  );
}

function Metric({ label, value, onClick }: { label: string; value: string; onClick?: () => void }) {
  const content = (
    <>
      <span>{label}</span>
      <strong>{value}</strong>
    </>
  );
  return onClick ? (
    <button type="button" className="metric" onClick={onClick}>
      {content}
    </button>
  ) : (
    <div className="metric">{content}</div>
  );
}

function PageTitle({ children }: { children: ReactNode }) {
  return <h1 className="page-title">{children}</h1>}

function useHashPage(): [Page, (page: Page) => void] {
  const parse = (): Page => {
    const value = location.hash.replace("#", "") as Page;
    return ["dashboard", "products", "instances", "subscriptions", "orders", "invoices", "wallet", "notifications", "tickets", "account"].includes(value)
      ? value
      : "dashboard";
  };
  const [page, setPage] = useState<Page>(parse);
  useEffect(() => {
    const listener = () => setPage(parse());
    addEventListener("hashchange", listener);
    return () => removeEventListener("hashchange", listener);
  }, []);
  return [
    page,
    (next) => {
      location.hash = next;
      setPage(next);
    },
  ];
}

function localized(value: Record<string, string>, locale: Locale) {
  return value[locale] ?? value["en-US"] ?? Object.values(value)[0] ?? "";
}

function money(value: number, currency: string, locale: Locale) {
  return new Intl.NumberFormat(locale, { style: "currency", currency: currency || "USD" }).format(value / 100);
}

function dateTime(value: string | null, locale: Locale) {
  return value ? new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "—";
}

function formatBytes(value: number, locale: Locale) {
  return new Intl.NumberFormat(locale, { style: "unit", unit: "gigabyte", maximumFractionDigits: 2 }).format(value / 1_073_741_824);
}

function statusText(status: string, t: T) {
  const key = `status.${status}` as MessageKey;
  const translated = t(key);
  return translated === key ? status : translated;
}

function errorText(error: unknown, t: T) {
  if (error instanceof ApiRequestError) return t(error.messageKey as MessageKey);
  return t("errors.internal");
}

function isTerminal(status: Operation["status"] | undefined) {
  return status === "succeeded" || status === "failed" || status === "cancelled";
}

function safeTranslate(key: string, t: T) {
  try {
    return t(key as MessageKey);
  } catch {
    return key;
  }
}
