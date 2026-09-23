import { describe, expect, it } from "vitest";

import { translate } from "./index";
import { messages } from "./messages";

describe("translate", () => {
  it("returns messages for both supported locales", () => {
    expect(translate("zh-CN", "foundation.status")).toBe("基础服务已就绪");
    expect(translate("en-US", "foundation.status")).toBe(
      "Foundation services are ready",
    );
  });

  it("keeps the zh-CN and en-US catalogs structurally identical", () => {
    expect(Object.keys(messages["zh-CN"]).sort()).toEqual(Object.keys(messages["en-US"]).sort());
  });
});
