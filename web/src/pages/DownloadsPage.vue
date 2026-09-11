<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type Download } from '../api'
import { errorMessage, useNotify } from '../notify'
import { formatDateTime } from '../format'
import PageHeader from '../components/PageHeader.vue'
import { admin } from '../config'
import EmptyState from '../components/EmptyState.vue'

const notify = useNotify()
const rows = ref<Download[]>([])
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    rows.value = await api.downloads.list(300)
  } catch (err) {
    notify.error('加载下载记录失败', errorMessage(err))
  } finally {
    loading.value = false
  }
}
onMounted(load)

/** "uv/0.9.3 {json…}" → "uv/0.9.3"; "pip/24.0 {json…}" → "pip/24.0". */
function shortAgent(ua: string): string {
  return ua.split(' ')[0] ?? ua
}
</script>

<template>
  <div>
    <PageHeader title="下载记录" description="每次文件下载一行（不含 metadata 请求）。同一 token 短时间多个 IP 通常意味着 token 被分享了。">
      <UButton label="刷新" icon="i-lucide-refresh-cw" color="neutral" variant="ghost" :loading="loading" @click="load" />
    </PageHeader>

    <EmptyState v-if="!loading && rows.length === 0" icon="i-lucide-download" title="还没有下载记录" />

    <div v-else class="rounded-lg border border-default overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-elevated/50 text-left text-xs uppercase text-muted">
          <tr>
            <th class="px-4 py-2 font-medium">时间</th>
            <th class="px-4 py-2 font-medium">用户</th>
            <th class="px-4 py-2 font-medium">包</th>
            <th class="px-4 py-2 font-medium">文件</th>
            <th class="px-4 py-2 font-medium">IP</th>
            <th class="px-4 py-2 font-medium">客户端</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-default">
          <tr v-for="d in rows" :key="d.id">
            <td class="px-4 py-2 text-muted whitespace-nowrap">{{ formatDateTime(d.at) }}</td>
            <td class="px-4 py-2">{{ d.username || '（已删除）' }}</td>
            <td class="px-4 py-2">
              <RouterLink v-if="d.package_name" :to="admin(`/packages/${d.package_name}`)" class="text-primary hover:underline">{{ d.package_name }}</RouterLink>
              <span v-else class="text-muted">—</span>
            </td>
            <td class="px-4 py-2 font-mono text-xs break-all">{{ d.filename }}</td>
            <td class="px-4 py-2 font-mono text-xs">{{ d.ip }}</td>
            <td class="px-4 py-2 text-xs text-muted" :title="d.user_agent">{{ shortAgent(d.user_agent) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
