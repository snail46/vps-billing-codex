export interface ApiSuccess<T> {
  success: true;
  data: T;
  request_id: string;
}

export interface ApiErrorDetail {
  code: string;
  message_key: string;
  details?: Record<string, unknown>;
}

export interface ApiFailure {
  success: false;
  error: ApiErrorDetail;
  request_id: string;
}

export type ApiResponse<T> = ApiSuccess<T> | ApiFailure;

export type HealthStatus = "alive" | "ready" | "not_ready";

export interface HealthData {
  status: HealthStatus;
  checks?: Record<string, "up" | "down">;
}

export interface UserPrincipal {
  id: string;
  email: string;
  status: string;
  locale: "zh-CN" | "en-US";
  timezone: string;
}

export interface AdminPrincipal {
  id: string;
  email: string;
  status: string;
  display_name: string;
  two_factor_enabled: boolean;
  permissions: string[];
}

export interface AuthData<T> {
  principal: T;
  csrf_token: string;
}

export interface TOTPSetup {
  secret: string;
  otpauth_uri: string;
}

export interface CatalogItem {
  product_id: string; product_slug: string; product_name_i18n: Record<string, string>; description_i18n: Record<string, string>;
  plan_id: string; plan_slug: string; plan_name_i18n: Record<string, string>; cpu_cores: number; memory_mb: number; disk_gb: number;
  traffic_gb: number | null; bandwidth_mbps: number | null; ipv4_count: number; ipv6_count: number; nat_port_count: number;
  virtualization: string; billing_cycle: string; price_minor: number; currency: string;
}

export interface Order { id: string; order_no: string; status: string; total_minor: number; currency: string; kind: string; payment_id?: string; payment_status?: string; subscription_id?: string }
export interface Invoice { id: string; invoice_no: string; status: string; amount_minor: number; currency: string; due_at: string; subscription_id?: string; order_id?: string }
export interface Wallet { id: string; currency: string; available_balance_minor: number }
export interface ItemList<T> { items: T[] }

export interface Instance {
  id: string; name: string; desired_state: string; observed_state: string; cpu_cores: number; memory_mb: number; disk_gb: number;
  traffic_limit_gb: number | null; bandwidth_mbps: number | null; image_id: string | null; primary_ipv4: string; primary_ipv6: string;
  last_synced_at: string | null; created_at: string; updated_at: string; subscription_id: string; subscription_status: string;
  current_period_end: string | null; plan_slug: string; plan_name_i18n: Record<string, string>;
}
export interface InstanceNetwork { id: string; type: string; address: string; gateway: string; prefix: number | null; created_at: string }
export interface TrafficRecord { period_start: string; period_end: string; rx_bytes: number; tx_bytes: number; source: string }
export interface Notification { id: string; type: string; title_key: string; message_key: string; parameters: Record<string, unknown>; severity: string; read_at: string | null; created_at: string }
export interface TicketMessage { id: string; sender_type: string; message: string; created_at: string }
export interface Ticket { id: string; ticket_no: string; subject: string; status: string; priority: string; created_at: string; updated_at: string; closed_at: string | null; messages?: TicketMessage[] }
export interface OperationAccepted { operation_id: string; status: "queued" }

export type SubscriptionStatus =
  | "pending"
  | "active"
  | "past_due"
  | "suspended"
  | "cancelled"
  | "expired"
  | "terminated";

export interface Subscription {
  id: string;
  plan_id: string;
  plan_slug: string;
  plan_name_i18n: Record<string, string>;
  status: SubscriptionStatus;
  billing_cycle: "monthly" | "quarterly" | "yearly";
  price_minor: number;
  currency: string;
  started_at: string | null;
  current_period_start: string | null;
  current_period_end: string | null;
  next_due_at: string | null;
  grace_until: string | null;
  cancel_at_period_end: boolean;
  ended_at: string | null;
  version: number;
}

export type OperationStatus =
  | "queued"
  | "running"
  | "waiting_provider"
  | "waiting_resource"
  | "verifying"
  | "retrying"
  | "succeeded"
  | "failed"
  | "cancelled";

export interface OperationStep {
  key: string;
  order: number;
  status: "pending" | "running" | "waiting" | "succeeded" | "failed" | "skipped";
  progress: number;
  attempt: number;
  error_code: string | null;
  output?: Record<string, unknown>;
  started_at: string | null;
  finished_at: string | null;
}

export interface Operation {
  id: string;
  type: string;
  resource_type: string;
  resource_id: string;
  status: OperationStatus;
  phase: string | null;
  progress: number;
  message_key: string | null;
  retryable: boolean;
  retry_count: number;
  max_retries: number;
  error_code: string | null;
  trace_id: string;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
  updated_at: string;
  steps: OperationStep[];
}

export class ApiRequestError extends Error {
  constructor(public readonly code: string, public readonly messageKey: string, public readonly status: number, public readonly details?: Record<string, unknown>) {
    super(code);
  }
}

export async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  const payload = (await response.json()) as ApiResponse<T>;
  if (!payload.success) {
    throw new ApiRequestError(payload.error.code, payload.error.message_key, response.status, payload.error.details);
  }
  return payload.data;
}
