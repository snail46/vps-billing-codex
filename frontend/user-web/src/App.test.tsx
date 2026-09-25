import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AuthScreen } from "./App";

describe("user web auth screen", () => {
  it("renders login form with email, password and submit button in en-US", () => {
    const queryClient = new QueryClient();
    const html = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AuthScreen locale="en-US" setLocale={() => undefined} t={(key) => key} onAuthenticated={() => undefined} />
      </QueryClientProvider>,
    );

    expect(html).toContain('type="email"');
    expect(html).toContain('type="password"');
    expect(html).toContain('minLength="12"');
    expect(html).toContain('maxLength="128"');
    expect(html).toContain('<button class="button" type="submit">auth.login</button>');
    expect(html).toContain('auth.switchToRegister');
  });

  it("renders in zh-CN locale without error", () => {
    const queryClient = new QueryClient();
    const html = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AuthScreen locale="zh-CN" setLocale={() => undefined} t={(key) => key} onAuthenticated={() => undefined} />
      </QueryClientProvider>,
    );

    expect(html).toContain('<button class="button" type="submit">auth.login</button>');
  });
});
