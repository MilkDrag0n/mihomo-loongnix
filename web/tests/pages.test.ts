import { mount, flushPromises } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { pending } from "../src/composables/useAction";
const { api } = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("../src/api/client", () => ({ api }));
import Rules from "../src/pages/Rules.vue";
import Profiles from "../src/pages/Profiles.vue";
import Proxies from "../src/pages/Proxies.vue";
beforeEach(() => {
  api.mockReset();
  pending.value = false;
});
describe("网页业务交互", () => {
  it("筛选规则重置页码并保持只读", async () => {
    api.mockResolvedValue(
      Array.from({ length: 35 }, (_, i) => ({
        content: "example" + i,
        type: "Domain",
        policy: "Auto",
      })),
    );
    const w = mount(Rules);
    await flushPromises();
    expect(w.text()).toContain("1 / 2");
    await w
      .findAll("button")
      .find((b) => b.text() === "下一页")!
      .trigger("click");
    expect(w.text()).toContain("2 / 2");
    await w.find("input").setValue("example1");
    expect(w.text()).toContain("1 / 1");
    expect(api).toHaveBeenCalledTimes(1);
    w.unmount();
  });
  it("导入链接不保留在输入框并回读列表", async () => {
    api.mockResolvedValue([]);
    const w = mount(Profiles);
    await flushPromises();
    await w
      .findAll("button")
      .find((b) => b.text().includes("导入配置"))!
      .trigger("click");
    await w.find("input[type=url]").setValue("https://example.invalid/sub");
    await w.find("form").trigger("submit");
    await flushPromises();
    expect(api).toHaveBeenCalledWith("/profiles", "POST", {
      name: "",
      url: "https://example.invalid/sub",
    });
    expect(w.find("input[type=url]").exists()).toBe(false);
    w.unmount();
  });
  it("切换含斜线和百分号的组名保持原始节点名", async () => {
    api.mockResolvedValue([
      {
        name: "中文/100%",
        type: "Selector",
        now: "old",
        nodes: [{ name: "节点/100%", type: "ss", delay: -3 }],
      },
    ]);
    const w = mount(Proxies);
    await flushPromises();
    await w.find(".node-title").trigger("click");
    await flushPromises();
    expect(api).toHaveBeenCalledWith(
      "/proxy-groups/" + encodeURIComponent("中文/100%"),
      "PUT",
      { name: "节点/100%" },
    );
    w.unmount();
  });
});
