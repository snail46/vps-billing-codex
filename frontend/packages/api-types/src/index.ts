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

export class ApiRequestError extends Error {
  constructor(public readonly code: string, public readonly messageKey: string) {
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
    throw new ApiRequestError(payload.error.code, payload.error.message_key);
  }
  return payload.data;
}
