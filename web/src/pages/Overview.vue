<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { api } from "../api/client";
import { action, pending } from "../composables/useAction";
import type { Status } from "../types";
const state = ref<Status>(),
  error = ref(""),
  port = ref(7890),
  portEdited = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined,
  alive = true;
async function refresh() {
  try {
    state.value = await api<Status>("/status");
    if (!portEdited.value) port.value = state.value.proxy_port;
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function tick() {
  if (!document.hidden) await refresh();
  if (alive) timer = setTimeout(tick, 3000);
}
onMounted(tick);
onUnmounted(() => {
  alive = false;
  clearTimeout(timer);
});
async function core() {
  await action(
    () =>
      api(
        "/core/" + (state.value?.core.service_active ? "stop" : "start"),
        "POST",
      ),
    refresh,
  );
}
async function tun() {
  if (
    !confirm(
      state.value?.tun.configured
        ? "关闭 TUN 将停止系统流量接管。继续吗？"
        : "开启 TUN 将接管系统流量；代理停止时仅保存配置。继续吗？",
    )
  )
    return;
  await action(
    () => api("/tun", "PUT", { enabled: !state.value?.tun.configured }),
    refresh,
  );
}
async function savePort() {
  if (!confirm("修改端口后，使用旧代理端口的客户端需要更新设置。继续吗？"))
    return;
  await action(
    () => api("/proxy-port", "PUT", { port: port.value }),
    async () => {
      portEdited.value = false;
      await refresh();
    },
  );
}
</script>
<template>
  <section>
    <div class="page-heading">
      <div>
        <span class="eyebrow">连接一目了然</span>
        <h1>网络概览<span class="title-dot">.</span></h1>
        <p class="muted">查看代理状态，掌握当前连接。</p>
      </div>
      <button class="btn btn-outline" @click="refresh">↻ 刷新状态</button>
    </div>
    <div v-if="error" class="notice error" role="alert">
      {{ error }} · 以下为最近一次状态
    </div>
    <div v-if="state" class="overview-grid">
      <article class="panel core-panel">
        <div class="panel-top">
          <span class="eyebrow">代理内核</span
          ><span
            class="status-pill"
            :class="{ online: state.core.running && !error }"
            >{{
              error
                ? "状态待确认"
                : state.core.state_query_ok === false
                  ? "状态未知"
                  : state.core.running
                    ? "运行中"
                    : state.core.service_active
                      ? "控制接口异常"
                      : "未运行"
            }}</span
          >
        </div>
        <div class="core-orb" :class="{ active: state.core.running }">
          <span>↗</span>
        </div>
        <h2>
          {{
            state.core.state_query_ok === false
              ? "状态待确认"
              : state.core.running
                ? "连接已就绪"
                : "等待连接"
          }}
        </h2>
        <p class="muted">
          {{
            state.core.running
              ? "代理服务与控制接口正常响应"
              : "开启代理，恢复网络连接"
          }}
        </p>
        <div class="core-facts">
          <span
            >系统服务
            <b>{{ state.core.service_active ? "运行中" : "未运行" }}</b></span
          ><span
            >控制接口
            <b>{{
              state.core.controller_healthy ? "可连接" : "不可连接"
            }}</b></span
          >
        </div>
        <button
          class="btn"
          :class="state.core.service_active ? 'btn-outline' : 'btn-primary'"
          :disabled="pending || !!error"
          @click="core"
        >
          {{ state.core.service_active ? "停止代理" : "启动代理" }}
        </button>
      </article>
      <article class="panel connection-panel">
        <div class="panel-top">
          <span class="eyebrow">当前连接</span
          ><span class="small muted">HTTP / SOCKS</span>
        </div>
        <div class="connection-address">
          127.0.0.1<span>:{{ state.proxy_port }}</span>
        </div>
        <dl class="detail-list">
          <div>
            <dt>当前节点</dt>
            <dd>{{ state.current_node || "尚未选择" }}</dd>
          </div>
          <div>
            <dt>代理组</dt>
            <dd>{{ state.current_group || "暂无" }}</dd>
          </div>
          <div>
            <dt>活动配置</dt>
            <dd>{{ state.active_profile?.name || "尚未导入" }}</dd>
          </div>
        </dl>
        <RouterLink class="text-link" to="/proxies"
          >管理代理节点 <span>→</span></RouterLink
        >
      </article>
      <article class="panel tun-panel">
        <div class="panel-top">
          <span class="eyebrow">TUN 网络</span
          ><span class="status-pill" :class="{ online: state.tun.enabled }">{{
            state.tun.enabled
              ? "已生效"
              : state.tun.configured
                ? "待生效"
                : "已关闭"
          }}</span>
        </div>
        <h2>系统流量接管</h2>
        <p class="muted">让应用通过虚拟网卡使用代理。</p>
        <div class="two-facts">
          <div>
            <span>期望配置</span
            ><strong>{{ state.tun.configured ? "开启" : "关闭" }}</strong>
          </div>
          <div>
            <span>虚拟网卡</span
            ><strong>{{
              state.tun.interface_present ? "已创建" : "未创建"
            }}</strong>
          </div>
        </div>
        <button
          class="btn btn-outline"
          :disabled="pending || !!error"
          @click="tun"
        >
          {{ state.tun.configured ? "关闭 TUN" : "开启 TUN" }}
        </button>
      </article>
      <article class="panel port-panel">
        <span class="eyebrow">连接设置</span>
        <h2>代理端口</h2>
        <p class="muted">HTTP 与 SOCKS 共用混合代理端口。</p>
        <form class="inline-form" @submit.prevent="savePort">
          <label class="sr-only" for="port">代理端口</label
          ><input
            id="port"
            v-model.number="port"
            type="number"
            min="1"
            max="65535"
            required
            class="input input-bordered"
            @input="portEdited = true"
          /><button class="btn btn-primary" :disabled="pending || !!error">
            应用端口
          </button>
        </form>
        <p class="small muted">仅修改代理端口，不修改网页访问端口。</p>
      </article>
    </div>
    <div v-else class="empty panel">
      {{ error ? "暂时无法读取管理器状态" : "正在读取连接状态…" }}
    </div>
  </section>
</template>
