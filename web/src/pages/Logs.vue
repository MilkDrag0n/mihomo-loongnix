<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import { api, session } from "../api/client";
import { action, pending } from "../composables/useAction";
import type { Log, Logging } from "../types";
const logs = ref<Log[]>([]),
  level = ref("info"),
  paused = ref(false),
  connection = ref("正在连接"),
  logging = ref<Logging>(),
  error = ref("");
let stream: EventSource | undefined,
  timer: ReturnType<typeof setTimeout> | undefined,
  alive = true,
  backoff = 1000;
async function refresh() {
  try {
    logging.value = await api<Logging>("/logging/status");
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  }
}
function add(log: Log) {
  if (paused.value) return;
  logs.value.push(log);
  if (logs.value.length > 1000) logs.value.splice(0, logs.value.length - 1000);
}
function connect() {
  stream?.close();
  clearTimeout(timer);
  if (!alive || document.hidden) return;
  connection.value = "正在连接";
  stream = new EventSource("/api/v1/logs/stream?level=" + level.value);
  stream.onopen = () => {
    connection.value = "实时连接";
    backoff = 1000;
  };
  stream.addEventListener("log", (e) => {
    try {
      add(JSON.parse((e as MessageEvent).data));
    } catch {}
  });
  stream.addEventListener("gap", () =>
    add({
      level: "提示",
      message: "日志连接有间断，可能遗漏部分记录。",
      received_at: new Date().toISOString(),
    }),
  );
  stream.onerror = async () => {
    stream?.close();
    connection.value = "连接已断开";
    try {
      await session();
    } catch {
      return;
    }
    if (alive) {
      timer = setTimeout(connect, backoff);
      backoff = Math.min(30000, backoff * 2);
    }
  };
}
function visibility() {
  if (document.hidden) {
    stream?.close();
    clearTimeout(timer);
    connection.value = "后台已暂停";
  } else {
    add({
      level: "提示",
      message: "页面恢复，后台期间的日志不补发。",
      received_at: new Date().toISOString(),
    });
    connect();
  }
}
async function toggle() {
  await action(
    () => api("/logging", "PUT", { enabled: !logging.value?.enabled }),
    refresh,
  );
}
watch(level, connect);
onMounted(() => {
  refresh();
  connect();
  document.addEventListener("visibilitychange", visibility);
});
onUnmounted(() => {
  alive = false;
  stream?.close();
  clearTimeout(timer);
  document.removeEventListener("visibilitychange", visibility);
});
</script>
<template>
  <section>
    <div class="page-heading">
      <div>
        <span class="eyebrow">观察每一次连接</span>
        <h1>运行日志<span class="title-dot">.</span></h1>
        <p class="muted">实时日志仅保留当前页面最近 1000 条。</p>
      </div>
      <span
        class="status-pill"
        :class="{ online: connection === '实时连接' }"
        >{{ connection }}</span
      >
    </div>
    <div v-if="error" class="notice error" role="alert">{{ error }}</div>
    <div class="panel logging-bar">
      <div>
        <h3>保存到磁盘</h3>
        <p class="small muted">
          {{ logging?.enabled ? "已启用" : "未启用" }} · 已用
          {{ ((logging?.total_bytes || 0) / 1048576).toFixed(2) }} MiB
          <span v-if="logging?.has_error" class="error"
            >· 记录器异常，请检查服务器日志</span
          >
        </p>
      </div>
      <button
        class="btn btn-outline btn-sm"
        :disabled="pending || !logging"
        @click="toggle"
      >
        {{ logging?.enabled ? "关闭记录" : "开启记录" }}
      </button>
    </div>
    <div class="panel log-panel">
      <div class="table-toolbar">
        <label class="actions small"
          >日志级别<select
            v-model="level"
            class="select select-bordered select-sm"
          >
            <option value="debug">调试</option>
            <option value="info">信息</option>
            <option value="warning">警告</option>
            <option value="error">错误</option>
          </select></label
        >
        <div class="actions">
          <button class="btn btn-sm btn-ghost" @click="paused = !paused">
            {{ paused ? "继续显示" : "暂停显示" }}</button
          ><button class="btn btn-sm btn-ghost" @click="logs = []">
            清空显示
          </button>
        </div>
      </div>
      <div class="log-lines" role="log" aria-label="运行日志">
        <div v-for="(l, i) in logs" :key="i" class="log-line">
          <time>{{ new Date(l.received_at).toLocaleTimeString("zh-CN") }}</time
          ><span class="log-level" :class="l.level">{{ l.level }}</span
          ><span>{{ l.message }}</span>
        </div>
        <div v-if="!logs.length" class="empty">
          {{ paused ? "显示已暂停" : "正在等待新的日志记录…" }}
        </div>
      </div>
      <div class="log-foot small muted">
        {{ logs.length }} 条记录 · 清空显示不会删除磁盘日志
      </div>
    </div>
  </section>
</template>
