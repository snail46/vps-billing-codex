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
