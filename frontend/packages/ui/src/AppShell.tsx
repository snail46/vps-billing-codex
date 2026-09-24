import type { PropsWithChildren, ReactNode } from "react";

interface AppShellProps extends PropsWithChildren {
  title: string;
  actions?: ReactNode;
}

export function AppShell({ title, actions, children }: AppShellProps) {
  return (
    <div className="app-shell">
      <header className="app-header">
        <span className="app-wordmark">{title}</span>
        <div className="app-actions">{actions}</div>
      </header>
      <main className="app-main">{children}</main>
    </div>
  );
}
