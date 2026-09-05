<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api/client";
import { action, pending } from "../composables/useAction";
import type { Profile } from "../types";
const profiles = ref<Profile[]>([]),
  error = ref(""),
  name = ref(""),
  url = ref(""),
  importOpen = ref(false);
async function refresh() {
  try {
    profiles.value = (await api<Profile[]>("/profiles")) || [];
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  }
}
onMounted(refresh);
async function importProfile() {
  const input = { name: name.value, url: url.value };
  url.value = "";
  await action(() => api("/profiles", "POST", input), refresh);
  importOpen.value = false;
  name.value = "";
}
async function activate(p: Profile) {
  await action(
    () => api("/profiles/" + encodeURIComponent(p.id) + "/activate", "POST"),
    refresh,
  );
}
async function update(p: Profile) {
  await action(
    () => api("/profiles/" + encodeURIComponent(p.id) + "/update", "POST"),
    refresh,
  );
}
async function rename(p: Profile) {
  const next = prompt("新的配置名称", p.name);
  if (!next?.trim()) return;
  await action(
    () => api("/profiles/" + encodeURIComponent(p.id), "PATCH", { name: next }),
    refresh,
  );
}
async function remove(p: Profile) {
  if (
    !confirm(
      "删除配置“" +
        p.name +
        "”？" +
        (p.active ? "这是当前活动配置，删除可能改变代理连接。" : ""),
    )
  )
    return;
  await action(
    () => api("/profiles/" + encodeURIComponent(p.id), "DELETE"),
    refresh,
  );
}
</script>
<template>
  <section>
    <div class="page-heading">
      <div>
        <span class="eyebrow">你的连接来源</span>
        <h1>配置管理<span class="title-dot">.</span></h1>
        <p class="muted">导入订阅，选择当前使用的代理配置。</p>
      </div>
      <button class="btn btn-primary" @click="importOpen = !importOpen">
        ＋ 导入配置
      </button>
    </div>
    <form
      v-if="importOpen"
      class="panel import-form"
      @submit.prevent="importProfile"
    >
      <h2>从订阅链接导入</h2>
      <label
        >配置名称<input
          v-model="name"
          class="input input-bordered"
          maxlength="120"
          placeholder="例如：日常使用" /></label
      ><label
        >订阅链接<input
          v-model="url"
          class="input input-bordered"
          type="url"
          required
          autocomplete="off"
          placeholder="https://…"
      /></label>
      <p class="small muted">链接仅用于本次导入，不保存在浏览器中。</p>
      <div class="actions">
        <button class="btn btn-primary" :disabled="pending">导入</button
        ><button
          class="btn btn-ghost"
          type="button"
          @click="
            url = '';
            importOpen = false;
          "
        >
          取消
        </button>
      </div>
    </form>
    <div v-if="error" class="notice error" role="alert">
      {{ error }}<button @click="refresh">重试</button>
    </div>
    <div class="section-label">
      <span>{{ profiles.length }} 份配置</span
      ><button class="text-link" @click="refresh">↻ 刷新</button>
    </div>
    <div class="profile-grid">
      <article
        v-for="p in profiles"
        :key="p.id"
        class="panel profile-card"
        :class="{ selected: p.active }"
      >
        <div class="panel-top">
          <span class="file-icon">≡</span
          ><span v-if="p.active" class="status-pill online">当前使用</span
          ><span v-else class="status-pill">未激活</span>
        </div>
        <h2>{{ p.name }}</h2>
        <p class="muted break-word">{{ p.source }}</p>
        <p class="small muted">更新于 {{ p.updated_at || "暂无记录" }}</p>
        <div class="actions wrap">
          <button
            class="btn btn-sm"
            :class="p.active ? 'btn-ghost' : 'btn-primary'"
            :disabled="pending || p.active"
            @click="activate(p)"
          >
            {{ p.active ? "已激活" : "使用此配置" }}</button
          ><button
            class="btn btn-sm btn-outline"
            :disabled="pending"
            @click="update(p)"
          >
            更新</button
          ><button
            class="btn btn-sm btn-ghost"
            :disabled="pending"
            @click="rename(p)"
          >
            重命名</button
          ><button
            class="btn btn-sm btn-ghost danger"
            :disabled="pending"
            @click="remove(p)"
          >
            删除
          </button>
        </div>
      </article>
    </div>
    <div v-if="!profiles.length && !error" class="panel empty">
      还没有配置，先导入一条订阅链接。
    </div>
  </section>
</template>
