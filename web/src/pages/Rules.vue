<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { api } from "../api/client";
import type { Rule } from "../types";
const rules = ref<Rule[]>([]),
  query = ref(""),
  page = ref(1),
  error = ref("");
const filtered = computed(() =>
    rules.value.filter((r) =>
      (r.type + " " + r.content + " " + r.policy)
        .toLowerCase()
        .includes(query.value.toLowerCase()),
    ),
  ),
  pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / 30))),
  visible = computed(() =>
    filtered.value.slice((page.value - 1) * 30, page.value * 30),
  );
watch(query, () => (page.value = 1));
async function refresh() {
  try {
    rules.value = (await api<Rule[]>("/rules")) || [];
    page.value = Math.min(page.value, pages.value);
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  }
}
onMounted(refresh);
</script>
<template>
  <section>
    <div class="page-heading">
      <div>
        <span class="eyebrow">流量去向，有据可循</span>
        <h1>生效规则<span class="title-dot">.</span></h1>
        <p class="muted">来自内核的当前规则，仅供查看。</p>
      </div>
      <button class="btn btn-outline" @click="refresh">↻ 刷新规则</button>
    </div>
    <div v-if="error" class="notice error" role="alert">{{ error }}</div>
    <div class="panel rules-panel">
      <div class="table-toolbar">
        <label class="search-label"
          ><span class="sr-only">筛选规则</span
          ><input
            v-model="query"
            class="input input-bordered"
            placeholder="搜索域名、规则类型或策略" /></label
        ><span class="muted small"
          >{{ filtered.length }} / {{ rules.length }} 条规则</span
        >
      </div>
      <div class="table-scroll">
        <table class="table">
          <thead>
            <tr>
              <th>序号</th>
              <th>类型</th>
              <th>匹配内容</th>
              <th>策略</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(r, i) in visible" :key="i">
              <td class="muted mono">{{ (page - 1) * 30 + i + 1 }}</td>
              <td>
                <span class="node-type">{{ r.type }}</span>
              </td>
              <td class="rule-content">{{ r.content || "所有剩余流量" }}</td>
              <td>
                <span class="policy">{{ r.policy }}</span>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="!visible.length" class="empty">暂无匹配规则</div>
      </div>
      <div class="pagination">
        <button
          class="btn btn-sm btn-outline"
          :disabled="page <= 1"
          @click="page--"
        >
          上一页</button
        ><span>{{ page }} / {{ pages }}</span
        ><button
          class="btn btn-sm btn-outline"
          :disabled="page >= pages"
          @click="page++"
        >
          下一页
        </button>
      </div>
    </div>
  </section>
</template>
