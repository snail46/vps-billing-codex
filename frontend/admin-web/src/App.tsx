import { ApiRequestError, apiRequest, type AdminPrincipal, type AuthData } from "@vps-billing/api-types";
import { type Locale, type MessageKey, useI18n } from "@vps-billing/i18n";
import { AppShell, Button, Card } from "@vps-billing/ui";
import { useMutation } from "@tanstack/react-query";
import { type FormEvent, useEffect, useState } from "react";

export function App() {
  const { locale, setLocale, t } = useI18n();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const mutation = useMutation({ mutationFn: () => apiRequest<AuthData<AdminPrincipal>>("/api/v1/admin/auth/login", { method: "POST", body: JSON.stringify({ email, password, totp_code: totpCode }) }) });

  useEffect(() => { document.documentElement.lang = locale; document.title = t("admin.appName"); }, [locale, t]);
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  const errorKey: MessageKey = mutation.error instanceof ApiRequestError ? mutation.error.messageKey as MessageKey : "errors.internal";

  return (
    <AppShell title={t("admin.appName")} actions={<label><span className="sr-only">{t("common.language")}</span><select className="locale-select" value={locale} onChange={(event) => setLocale(event.target.value as Locale)}><option value="zh-CN">{t("common.locale.zh-CN")}</option><option value="en-US">{t("common.locale.en-US")}</option></select></label>}>
      <div className="auth-card"><Card>
        <h1 className="foundation-title">{t("auth.adminTitle")}</h1>
        <form className="auth-form" onSubmit={submit}>
          <label className="form-field">{t("auth.email")}<input className="form-input" type="email" autoComplete="username" required value={email} onChange={(event) => setEmail(event.target.value)} /></label>
          <label className="form-field">{t("auth.password")}<input className="form-input" type="password" autoComplete="current-password" required value={password} onChange={(event) => setPassword(event.target.value)} /></label>
          <label className="form-field">{t("auth.totp")}<input className="form-input" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" value={totpCode} onChange={(event) => setTotpCode(event.target.value)} /></label>
          {mutation.isError && <p className="form-error" role="alert">{t(errorKey)}</p>}
          {mutation.isSuccess && <p className="form-success" role="status">{t("auth.success")}</p>}
          <Button type="submit" disabled={mutation.isPending}>{t(mutation.isPending ? "auth.pending" : "auth.login")}</Button>
        </form>
      </Card></div>
    </AppShell>
  );
}
