import { ApiRequestError, apiRequest, type AuthData, type UserPrincipal } from "@vps-billing/api-types";
import { type Locale, type MessageKey, useI18n } from "@vps-billing/i18n";
import { AppShell, Button, Card } from "@vps-billing/ui";
import { useMutation } from "@tanstack/react-query";
import { type FormEvent, useEffect, useState } from "react";

export function App() {
  const { locale, setLocale, t } = useI18n();
  const [register, setRegister] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const mutation = useMutation({
    mutationFn: () => apiRequest<AuthData<UserPrincipal>>(`/api/v1/auth/${register ? "register" : "login"}`, {
      method: "POST",
      body: JSON.stringify({ email, password, locale, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone }),
    }),
  });

  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = t("user.appName");
  }, [locale, t]);

  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  const errorKey: MessageKey = mutation.error instanceof ApiRequestError ? mutation.error.messageKey as MessageKey : "errors.internal";

  return (
    <AppShell title={t("user.appName")} actions={<LanguageSelect locale={locale} setLocale={setLocale} t={t} />}>
      <div className="auth-card"><Card>
        <h1 className="foundation-title">{t("auth.title")}</h1>
        <form className="auth-form" onSubmit={submit}>
          <label className="form-field">{t("auth.email")}<input className="form-input" type="email" autoComplete="email" required value={email} onChange={(event) => setEmail(event.target.value)} /></label>
          <label className="form-field">{t("auth.password")}<input className="form-input" type="password" autoComplete={register ? "new-password" : "current-password"} minLength={12} maxLength={128} required value={password} onChange={(event) => setPassword(event.target.value)} /></label>
          {mutation.isError && <p className="form-error" role="alert">{t(errorKey)}</p>}
          {mutation.isSuccess && <p className="form-success" role="status">{t("auth.success")}</p>}
          <Button type="submit" disabled={mutation.isPending}>{t(mutation.isPending ? "auth.pending" : register ? "auth.register" : "auth.login")}</Button>
          <button className="link-button" type="button" onClick={() => { setRegister(!register); mutation.reset(); }}>{t(register ? "auth.switchToLogin" : "auth.switchToRegister")}</button>
        </form>
      </Card></div>
    </AppShell>
  );
}

function LanguageSelect({ locale, setLocale, t }: { locale: Locale; setLocale: (locale: Locale) => void; t: (key: MessageKey) => string }) {
  return <label><span className="sr-only">{t("common.language")}</span><select className="locale-select" value={locale} onChange={(event) => setLocale(event.target.value as Locale)}><option value="zh-CN">{t("common.locale.zh-CN")}</option><option value="en-US">{t("common.locale.en-US")}</option></select></label>;
}
