<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { api } from "../api/client";
import { action, pending, notice, noticeError } from "../composables/useAction";
import type { Group, Node } from "../types";
const groups = ref<Group[]>([]),
  chosen = ref(""),
  query = ref(""),
  error = ref(""),
  testing = ref(""),
  batch = ref(false),
  tested = ref(0);
let alive = true;
const group = computed(() => groups.value.find((x) => x.name === chosen.value)),
  nodes = computed(() =>
    (group.value?.nodes || []).filter((n) =>
      (n.name + " " + n.type).toLowerCase().includes(query.value.toLowerCase()),
    ),
  );
async function refresh() {
  try {
    groups.value = (await api<Group[]>("/proxy-groups")) || [];
    if (!groups.value.some((x) => x.name === chosen.value))
      chosen.value = groups.value[0]?.name || "";
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  }
}
onMounted(refresh);
onUnmounted(() => {
  alive = false;
  batch.value = false;
});
async function select(n: Node) {
  await action(
    () =>
      api("/proxy-groups/" + encodeURIComponent(chosen.value), "PUT", {
        name: n.name,
      }),
    refresh,
  );
}
async function measure(n: Node, g = chosen.value) {
  if (testing.value) return;
  testing.value = n.name;
  try {
    const result = await api<{ delay: number }>("/proxy-delay", "POST", {
      group: g,
      name: n.name,
    });
    n.delay = result.delay;
  } catch (e) {
    n.delay = -1;
    noticeError.value = true;
    notice.value = (e as Error).message;
  } finally {
    testing.value = "";
  }
}
async function all() {
  if (batch.value) {
    batch.value = false;
    return;
  }
  batch.value = true;
  tested.value = 0;
  const snapshot = [...nodes.value],
    g = chosen.value;
  for (const n of snapshot) {
    if (!batch.value || !alive) break;
    await measure(n, g);
    tested.value++;
  }
  batch.value = false;
}
function latency(n: Node) {
  return testing.value === n.name
    ? "测试中…"
    : n.delay > 0
      ? n.delay + " ms"
      : n.delay === -3
        ? "未测试"
        : "超时";
}
</script>
<template>
  <section>
    <div class="page-heading">
      <div>
        <span class="eyebrow">找到合适的下一站</span>
        <h1>代理节点<span class="title-dot">.</span></h1>
        <p class="muted">选择代理组，查看延迟并切换连接。</p>
      </div>
      <button
        class="btn btn-outline"
        :disabled="!!testing || batch"
        @click="refresh"
      >
        ↻ 刷新节点
      </button>
    </div>
    <div v-if="error" class="notice error" role="alert">{{ error }}</div>
    <div class="panel node-toolbar">
      <label
        >代理组<select
          v-model="chosen"
          class="select select-bordered"
          :disabled="batch || !!testing"
        >
          <option v-for="g in groups" :key="g.name" :value="g.name">
            {{ g.name }} · {{ g.nodes?.length || 0 }} 节点
          </option>
        </select></label
      ><label class="search-label"
        >搜索节点<input
          v-model="query"
          class="input input-bordered"
          placeholder="名称或类型" /></label
      ><button
        class="btn"
        :class="batch ? 'btn-outline' : 'btn-primary'"
        :disabled="!group || (!batch && !!testing)"
        @click="all"
      >
        {{ batch ? "停止测速 · " + tested : "测速筛选结果" }}
      </button>
    </div>
    <div class="section-label">
      <span
        >{{ nodes.length }} 个节点
        <span class="muted">／ {{ group?.type || "暂无代理组" }}</span></span
      ><span class="small muted">{{
        batch
          ? "逐个测速，停止后不再发送剩余请求"
          : "当前：" + (group?.now || "未选择")
      }}</span>
    </div>
    <div class="node-grid">
      <article
        v-for="n in nodes"
        :key="n.name"
        class="panel node-card"
        :class="{ selected: group?.now === n.name }"
      >
        <div class="panel-top">
          <span class="node-type">{{ n.type }}</span
          ><span v-if="group?.now === n.name" class="selected-mark">✓ 当前</span
          ><span v-else class="muted">↗</span>
        </div>
        <button class="node-title" :disabled="pending" @click="select(n)">
          {{ n.name }}
        </button>
        <div class="node-bottom">
          <span
            class="latency"
            :class="
              n.delay > 0
                ? n.delay < 150
                  ? 'fast'
                  : n.delay < 500
                    ? 'medium'
                    : 'slow'
                : 'muted'
            "
            >{{ latency(n) }}</span
          ><button
            class="text-link"
            :disabled="!!testing || batch"
            @click="measure(n)"
          >
            测速</button
          ><button
            class="text-link"
            :disabled="pending || group?.now === n.name"
            @click="select(n)"
          >
            {{ group?.now === n.name ? "已选择" : "使用 →" }}
          </button>
        </div>
      </article>
    </div>
    <div v-if="!nodes.length" class="panel empty">
      {{ error ? "启动代理后重试，或检查控制接口。" : "没有符合条件的节点。" }}
    </div>
  </section>
</template>
