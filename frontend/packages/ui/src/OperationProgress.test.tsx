import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { Operation } from "@vps-billing/api-types";

import { OperationProgress } from "./OperationProgress";

describe("OperationProgress", () => {
  it("renders recoverable retry and step state using translated labels", () => {
    const operation: Operation = {
      id: "0199-0000-7000-8000-000000000001",
      type: "test",
      resource_type: "instance",
      resource_id: "0199-0000-7000-8000-000000000002",
      status: "retrying",
      phase: "allocate",
      progress: 25,
      message_key: "operation.retrying",
      retryable: true,
      retry_count: 1,
      max_retries: 3,
      error_code: "PROVIDER_TEMPORARY",
      trace_id: "trace-test",
      started_at: null,
      finished_at: null,
      created_at: "2026-09-23T00:00:00Z",
      updated_at: "2026-09-23T00:00:00Z",
      steps: [{ key: "allocate", order: 1, status: "running", progress: 25, attempt: 1, error_code: null, started_at: null, finished_at: null }],
      attempts: [{ attempt: 1, status: "running", error_code: null, started_at: "2026-09-23T00:00:00Z", finished_at: null }],
    };
    const html = renderToStaticMarkup(<OperationProgress operation={operation} translate={(key) => `translated:${key}`} />);

    expect(html).toContain("translated:operation.retrying");
    expect(html).toContain("PROVIDER_TEMPORARY");
    expect(html).toContain("aria-busy=\"true\"");
    expect(html).toContain("translated:operation.steps.allocate");
  });
});
