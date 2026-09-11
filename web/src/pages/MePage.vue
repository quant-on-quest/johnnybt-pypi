<script setup lang="ts">
import { reactive, ref } from 'vue'
import { api, type SelfCheck } from '../api'
import { errorMessage } from '../notify'
import { expiryLabel } from '../format'
import InstallSnippets from '../components/InstallSnippets.vue'

const state = reactive({ token: '' })
const result = ref<SelfCheck | null>(null)
const error = ref('')
const loading = ref(false)

async function onSubmit(e: { data: { token: string } }) {
  error.value = ''
  loading.value = true
  try {
    result.value = await api.me(e.data.token.trim())
    state.token = e.data.token.trim()
  } catch (err) {
    result.value = null
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-default text-default">
    <header class="border-b border-default">
      <div class="max-w-3xl mx-auto px-4 h-14 flex items-center gap-2 font-semibold text-highlighted">
        <UIcon name="i-lucide-box" class="size-5 text-primary" />
        johnnybt-pypi
        <span class="text-sm font-normal text-muted ml-2">客户自查</span>
      </div>
    </header>
    <main class="max-w-3xl mx-auto px-4 py-8 space-y-6">
      <UCard>
        <template #header>
          <h1 class="font-medium">粘贴你的 token，查看可安装的包和到期时间</h1>
        </template>
        <UForm :state="state" class="flex flex-wrap items-end gap-2" @submit="onSubmit">
          <UFormField label="Token" name="token" class="flex-1 min-w-64">
            <UInput v-model="state.token" placeholder="jbt_…" class="w-full font-mono" autocomplete="off" />
          </UFormField>
          <UButton type="submit" label="查询" icon="i-lucide-search" :loading="loading" />
        </UForm>
        <p v-if="error" class="text-sm text-error mt-3">{{ error }}</p>
        <p class="text-xs text-muted mt-3">token 只发送到本服务器做校验，不会被记录在页面里。</p>
      </UCard>

      <template v-if="result">
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <h2 class="font-medium">{{ result.username }}<span v-if="result.note" class="text-muted font-normal text-sm ml-2">{{ result.note }}</span></h2>
              <UBadge variant="subtle" color="neutral">token {{ result.token.prefix }}… · {{ result.token.scope }}</UBadge>
            </div>
          </template>
          <p v-if="result.packages.length === 0" class="text-sm text-muted">这个 token 还没有任何包的权限，请联系我们。</p>
          <table v-else class="w-full text-sm">
            <tbody class="divide-y divide-default">
              <tr v-for="p in result.packages" :key="p.normalized_name">
                <td class="py-2 font-medium">{{ p.package_name }}</td>
                <td class="py-2 font-mono text-muted">{{ p.latest_version || '—' }}</td>
                <td class="py-2 text-right">
                  <UBadge :color="expiryLabel(p.expires_at, p.active).color" variant="subtle" size="sm">{{ expiryLabel(p.expires_at, p.active).text }}</UBadge>
                </td>
              </tr>
            </tbody>
          </table>
        </UCard>

        <UCard v-if="result.packages.length > 0">
          <template #header><h2 class="font-medium">安装方式</h2></template>
          <InstallSnippets :index-url="result.index_url" :token="state.token" :package-name="result.packages[0]!.normalized_name" />
        </UCard>
      </template>
    </main>
  </div>
</template>
