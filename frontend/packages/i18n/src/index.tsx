import {
  createContext,
  type PropsWithChildren,
  useContext,
  useMemo,
  useState,
} from "react";

import { messages, type Locale, type MessageKey } from "./messages";

const defaultLocale: Locale = "zh-CN";

export function translate(locale: Locale, key: MessageKey): string {
  return messages[locale][key] ?? messages["en-US"][key];
}

interface I18nValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey) => string;
}

const I18nContext = createContext<I18nValue | null>(null);

export function I18nProvider({ children }: PropsWithChildren) {
  const [locale, setLocale] = useState<Locale>(defaultLocale);
  const value = useMemo<I18nValue>(
    () => ({ locale, setLocale, t: (key) => translate(locale, key) }),
    [locale],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) {
    throw new Error("useI18n must be used inside I18nProvider");
  }
  return value;
}

export type { Locale, MessageKey } from "./messages";
