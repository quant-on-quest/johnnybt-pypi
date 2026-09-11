<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type PackageSummary } from '../api'
import { errorMessage, useNotify } from '../notify'
import { formatDate } from '../format'
import PageHeader from '../components/PageHeader.vue'
import { admin } from '../config'
import EmptyState from '../components/EmptyState.vue'

const notify = useNotify()
const packages = ref<PackageSummary[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    packages.value = await api.packages.list()
  } catch (err) {
    notify.error('加载失败', errorMessage(err))
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div>
    <PageHeader title="包" description="所有已上传的包。客户只能看到你授权给他的那些。">
      <UButton :to="admin('/upload')" label="上传" icon="i-lucide-upload" />
    </PageHeader>

    <EmptyState v-if="!loading && packages.length === 0" icon="i-lucide-package" title="还没有包" description="用 uv publish 或右上角的上传按钮发布第一个版本。">
      <UButton :to="admin('/upload')" label="去上传" variant="subtle" />
    </EmptyState>

    <div v-else class="rounded-lg border border-default overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-elevated/50 text-left text-xs uppercase text-muted">
          <tr>
            <th class="px-4 py-2 font-medium">名称</th>
            <th class="px-4 py-2 font-medium">最新版本</th>
            <th class="px-4 py-2 font-medium text-right">版本数</th>
            <th class="px-4 py-2 font-medium text-right">文件</th>
            <th class="px-4 py-2 font-medium text-right">授权用户</th>
            <th class="px-4 py-2 font-medium text-right">下载</th>
            <th class="px-4 py-2 font-medium">更新于</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-default">
          <tr v-for="p in packages" :key="p.id" class="hover:bg-elevated/30">
            <td class="px-4 py-2">
              <RouterLink :to="admin(`/packages/${p.normalized_name}`)" class="font-medium text-primary hover:underline">{{ p.name }}</RouterLink>
              <p class="text-xs text-muted truncate max-w-md">{{ p.summary }}</p>
            </td>
            <td class="px-4 py-2 font-mono">{{ p.latest_version || '—' }}</td>
            <td class="px-4 py-2 text-right tabular-nums">{{ p.release_count }}</td>
            <td class="px-4 py-2 text-right tabular-nums">{{ p.file_count }}</td>
            <td class="px-4 py-2 text-right tabular-nums">{{ p.user_count }}</td>
            <td class="px-4 py-2 text-right tabular-nums">{{ p.download_count }}</td>
            <td class="px-4 py-2 text-muted">{{ formatDate(p.updated_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
