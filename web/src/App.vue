<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  api,
  authenticated,
  authMode,
  detectAuthMode,
  login,
  logout,
  session,
} from "./api/client";
import { notice, noticeError } from "./composables/useAction";
const route = useRoute(),
  router = useRouter(),
  ready = ref(false),
  password = ref(""),
  error = ref(""),
  busy = ref(false);
const theme = ref(
  localStorage.getItem("mihomo-theme") === "dark" ? "dark" : "light",
);
watch(
  theme,
  (t) => {
    document.documentElement.dataset.theme = t;
    localStorage.setItem("mihomo-theme", t);
  },
  { immediate: true },
);
const pages = [
  ["/overview", "概览", "01"],
  ["/profiles", "配置", "02"],
  ["/proxies", "节点", "03"],
  ["/rules", "规则", "04"],
  ["/logs", "日志", "05"],
];
async function signIn() {
  busy.value = true;
  error.value = "";
  try {
    if (authMode.value === null) await detectAuthMode();
    if (authMode.value === "external") await session();
    else {
      if (!password.value) return;
      await login(password.value);
    }
    if (route.path === "/login") await router.replace("/overview");
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    password.value = "";
    busy.value = false;
  }
}
async function signOut() {
  try {
    await logout();
  } catch (e) {
    noticeError.value = true;
    notice.value = (e as Error).message;
  }
}
let touched = 0;
function activity(e: Event) {
  if (!e.isTrusted || !authenticated.value || Date.now() - touched < 60000)
    return;
  touched = Date.now();
  api("/auth/refresh", "POST").catch(() => {});
}
onMounted(async () => {
  try {
    await detectAuthMode();
    await session();
  } catch (e) {
    if (authMode.value !== "password") error.value = (e as Error).message;
  } finally {
    ready.value = true;
  }
  window.addEventListener("pointerdown", activity);
  window.addEventListener("keydown", activity);
});
onUnmounted(() => {
  window.removeEventListener("pointerdown", activity);
  window.removeEventListener("keydown", activity);
});
watch(
  () => route.path,
  () => {
    notice.value = "";
  },
);
</script>
<template>
  <div v-if="!ready" class="loading-screen">正在连接控制台…</div>
  <main v-else-if="!authenticated" class="login-layout">
    <div class="login-brand">
      <div class="brand-mark">M</div>
      <span>MIHOMO</span>
    </div>
    <form class="login-card" @submit.prevent="signIn">
      <span class="eyebrow">你的网络，由你掌控</span>
      <h1>{{ authMode === "password" ? "欢迎回来。" : "连接控制台" }}</h1>
      <p class="muted">
        {{
          authMode === "password"
            ? "登录代理控制台，查看连接与管理节点。"
            : "通过外部访问验证后，无需额外的网页密码。"
        }}
      </p>
      <template v-if="authMode === 'password'">
        <label for="password">管理员密码</label
        ><input
          id="password"
          v-model="password"
          class="input input-bordered"
          type="password"
          required
          autocomplete="current-password"
          maxlength="1024"
          placeholder="输入管理员密码"
        />
      </template>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <button class="btn btn-primary" :disabled="busy">
        {{
          busy
            ? "正在连接…"
            : authMode === "password"
              ? "进入控制台 →"
              : "重新连接"
        }}
      </button>
      <p v-if="authMode === 'password'" class="login-note">
        使用服务器本机设置的密码
      </p>
    </form>
    <span class="login-foot">Mihomo Loongnix · 代理管理</span>
  </main>
  <div v-else class="app-shell">
    <aside class="sidebar">
      <RouterLink to="/overview" class="brand"
        ><span class="brand-mark">M</span
        ><span>MIHOMO<small>代理控制台</small></span></RouterLink
      >
      <div class="nav-label">工作空间</div>
      <nav aria-label="主导航">
        <RouterLink v-for="p in pages" :key="p[0]" :to="p[0]"
          ><span class="nav-number">{{ p[2] }}</span
          >{{ p[1] }}<span class="nav-arrow">↗</span></RouterLink
        >
      </nav>
      <div class="sidebar-bottom">
        <span class="muted small">Mihomo Loongnix</span
        ><span class="small">独立网页 · 共用配置</span>
      </div>
    </aside>
    <div class="main-column">
      <header class="topbar">
        <span class="breadcrumb"
          >工作空间 <span>/</span>
          {{ pages.find((p) => p[0] === route.path)?.[1] || "概览" }}</span
        >
        <div class="top-actions">
          <button
            class="btn btn-ghost btn-sm"
            @click="theme = theme === 'dark' ? 'light' : 'dark'"
          >
            {{ theme === "dark" ? "浅色模式" : "深色模式" }}</button
          ><span class="avatar-dot">管</span
          ><button
            v-if="authMode === 'password'"
            class="btn btn-ghost btn-sm"
            @click="signOut"
          >
            退出
          </button>
        </div>
      </header>
      <div class="page-content">
        <div
          v-if="notice"
          class="notice"
          :class="{ error: noticeError }"
          role="status"
        >
          {{ notice
          }}<button aria-label="关闭提示" @click="notice = ''">×</button>
        </div>
        <RouterView />
      </div>
      <footer class="app-footer">
        <span>MIHOMO / 代理控制台</span><span>关闭网页不会停止代理</span>
      </footer>
    </div>
  </div>
</template>
