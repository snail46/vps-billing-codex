import i18next, { type i18n as I18nInstance } from "i18next";
import { useCallback, type PropsWithChildren } from "react";
import { I18nextProvider, initReactI18next, useTranslation } from "react-i18next";

import { messages, type Locale, type MessageKey } from "./messages";

const defaultLocale: Locale = "zh-CN";

export const i18n: I18nInstance = i18next.createInstance();
export const i18nReady = i18n.use(initReactI18next).init({
  fallbackLng: "en-US",
  initAsync: false,
  interpolation: { escapeValue: false },
  lng: defaultLocale,
  resources: {
    "en-US": { translation: messages["en-US"] },
    "zh-CN": { translation: messages["zh-CN"] },
  },
});
void i18nReady.catch((error: unknown) => {
  console.error("i18n initialization failed", error);
});

export function translate(locale: Locale, key: MessageKey): string {
  return i18n.getFixedT(locale)(key);
}

interface I18nValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey) => string;
}

export function I18nProvider({ children }: PropsWithChildren) {
  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>;
}

export function useI18n(): I18nValue {
  const { i18n: activeInstance, t } = useTranslation();
  const locale: Locale = activeInstance.resolvedLanguage === "en-US" ? "en-US" : "zh-CN";
  const setLocale = useCallback((nextLocale: Locale) => {
    void activeInstance.changeLanguage(nextLocale);
  }, [activeInstance]);
  return {
    locale,
    setLocale,
    t: (key) => t(key),
  };
}

export type { Locale, MessageKey } from "./messages";
