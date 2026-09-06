import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import Logs from "../src/pages/Logs.vue";

vi.mock("../src/api/client", () => ({
  api: vi.fn(async () => ({ enabled: false, total_bytes: 0 })),
  session: vi.fn(async () => ({})),
}));

class DemoEventSource extends EventTarget {
  static instances: DemoEventSource[] = [];
  onopen?: () => void;
  onerror?: () => Promise<void>;
  close = vi.fn();
  constructor(_url: string) {
    super();
    DemoEventSource.instances.push(this);
  }
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(document, "hidden", "get").mockReturnValue(false);
  DemoEventSource.instances = [];
  vi.stubGlobal("EventSource", DemoEventSource);
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("keeps exponential retry across successful SSE headers with immediate upstream failure", async () => {
  const page = mount(Logs);
  try {
    await vi.advanceTimersByTimeAsync(0);
    const first = DemoEventSource.instances[0];
    first.onopen?.();
    await first.onerror?.();
    await vi.advanceTimersByTimeAsync(1000);
    expect(DemoEventSource.instances).toHaveLength(2);
    const second = DemoEventSource.instances[1];
    second.onopen?.();
    await second.onerror?.();
    await vi.advanceTimersByTimeAsync(1000);
    expect(DemoEventSource.instances).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(1000);
    expect(DemoEventSource.instances).toHaveLength(3);
    const third = DemoEventSource.instances[2];
    third.onopen?.();
    third.dispatchEvent(
      new MessageEvent("log", {
        data: JSON.stringify({
          level: "info",
          message: "测试日志",
          received_at: new Date().toISOString(),
        }),
      }),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(page.text()).toContain("实时连接");
    expect(page.text()).toContain("测试日志");
  } finally {
    page.unmount();
  }
  await vi.advanceTimersByTimeAsync(30000);
  expect(DemoEventSource.instances).toHaveLength(3);
});
