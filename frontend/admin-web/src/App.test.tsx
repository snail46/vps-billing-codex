import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Login } from "./App";

describe("admin login", () => {
  it("renders a submit button so clicking sign in submits the form", () => {
    const queryClient = new QueryClient();
    const html = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <Login locale="en-US" setLocale={() => undefined} t={(key) => key} success={() => undefined} />
      </QueryClientProvider>,
    );

    expect(html).toContain('<button class="button" type="submit">auth.login</button>');
  });
});
