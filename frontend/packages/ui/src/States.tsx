import type { PropsWithChildren, ReactNode } from "react";

export function StatusBadge({ status, label }: { status: string; label: string }) { return <span className="status-badge" data-status={status}>{label}</span>; }
export function Skeleton({ label }: { label: string }) { return <div className="state-panel" role="status"><span className="skeleton-line" />{label}</div>; }
export function EmptyState({ title, action }: { title: string; action?: ReactNode }) { return <div className="state-panel"><p>{title}</p>{action}</div>; }
export function ErrorState({ title, retry, retryLabel }: { title: string; retry?: () => void; retryLabel?: string }) { return <div className="state-panel state-panel--error" role="alert"><p>{title}</p>{retry && retryLabel && <button className="link-button" type="button" onClick={retry}>{retryLabel}</button>}</div>; }
export function Alert({ tone = "info", children }: PropsWithChildren<{ tone?: "info" | "warning" | "danger" | "success" }>) { return <div className="alert" data-tone={tone} role={tone === "danger" ? "alert" : "status"}>{children}</div>; }
