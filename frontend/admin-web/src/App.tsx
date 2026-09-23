import { type Locale, useI18n } from "@vps-billing/i18n";
import { AppShell, Card } from "@vps-billing/ui";
import { useEffect } from "react";

export function App() {
  const { locale, setLocale, t } = useI18n();

  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = t("admin.appName");
  }, [locale, t]);

  return (
    <AppShell
      title={t("admin.appName")}
      actions={
        <label>
          <span className="sr-only">{t("common.language")}</span>
          <select
            className="locale-select"
            value={locale}
            onChange={(event) => setLocale(event.target.value as Locale)}
          >
            <option value="zh-CN">{t("common.locale.zh-CN")}</option>
            <option value="en-US">{t("common.locale.en-US")}</option>
          </select>
        </label>
      }
    >
      <Card>
        <h1 className="foundation-title">{t("foundation.status")}</h1>
        <p className="foundation-copy">{t("foundation.description")}</p>
      </Card>
    </AppShell>
  );
}
