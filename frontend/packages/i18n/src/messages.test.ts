import { describe, expect, it } from "vitest";

import { translate } from "./index";

describe("translate", () => {
  it("returns messages for both supported locales", () => {
    expect(translate("zh-CN", "foundation.status")).toBe("基础服务已就绪");
    expect(translate("en-US", "foundation.status")).toBe(
      "Foundation services are ready",
    );
  });
});
