import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import App from "../src/App.vue";
import { authenticated, authMode, csrf } from "../src/api/client";

beforeEach(() => {
  authenticated.value = false;
  authMode.value = null;
  csrf.value = "";
});
afterEach(() => vi.unstubAllGlobals());

async function open(mode: "password" | "external") {
  const fetcher = vi.fn(async (path: string) => {
    if (path.endsWith("/auth/mode"))
      return new Response(JSON.stringify({ data: { auth_mode: mode } }));
    if (path.endsWith("/auth/session") && mode === "external")
      return new Response(
        JSON.stringify({ data: { csrf_token: "demo-csrf" } }),
      );
    return new Response(
      JSON.stringify({ error: { code: "UNAUTHORIZED", message: "请登录" } }),
      { status: 401 },
    );
  });
  vi.stubGlobal("fetch", fetcher);
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/:pathMatch(.*)*",
        component: { template: "<div>测试概览</div>" },
      },
    ],
  });
  await router.push("/overview");
  await router.isReady();
  const page = mount(App, { global: { plugins: [router] } });
  await flushPromises();
  return { page, fetcher };
}

it("opens the console directly with external access control and no password form", async () => {
  const { page, fetcher } = await open("external");
  try {
    expect(page.text()).toContain("测试概览");
    expect(page.find('input[type="password"]').exists()).toBe(false);
    expect(page.text()).not.toContain("管理员密码");
    expect(page.text()).not.toContain("退出");
    expect(csrf.value).toBe("demo-csrf");
    expect(
      fetcher.mock.calls.some(([path]) => path.endsWith("/auth/login")),
    ).toBe(false);
  } finally {
    page.unmount();
  }
});

it("preserves the password form for deployments that use password authentication", async () => {
  const { page } = await open("password");
  try {
    expect(page.find('input[type="password"]').exists()).toBe(true);
    expect(page.text()).toContain("管理员密码");
  } finally {
    page.unmount();
  }
});
