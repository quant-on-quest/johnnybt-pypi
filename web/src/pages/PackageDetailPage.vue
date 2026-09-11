<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, type PackageDetail } from '../api'
import { errorMessage, useNotify } from '../notify'
import { expiryLabel, formatBytes, formatDate } from '../format'
import PageHeader from '../components/PageHeader.vue'
import { admin } from '../config'
import InstallSnippets from '../components/InstallSnippets.vue'

const props = defineProps<{ name: string }>()
const notify = useNotify()
const router = useRouter()

const detail = ref<PackageDetail | null>(null)
const indexUrl = ref('')
const busy = ref(false)
const pkg = computed(() => detail.value?.package)

async function load() {
  try {
    detail.value = await api.packages.get(props.name)
  } catch (err) {
    notify.error('加载包失败', errorMessage(err))
  }
}
onMounted(async () => {
  await load()
  try {
    indexUrl.value = (await api.config()).index_url
  } catch {
    // snippets just fall back to a relative index url
  }
})
watch(() => props.name, load)

async function run(label: string, fn: () => Promise<unknown>) {
  busy.value = true
  try {
    await fn()
    notify.success(label)
    await load()
  } catch (err) {
    notify.error(`${label}失败`, errorMessage(err))
  } finally {
    busy.value = false
  }
}

function yank(version: string) {
  const reason = prompt(`标记 ${version} 为 yanked。原因（可选，会展示给安装方）：`)
  if (reason === null) return
  return run(`已 yank ${version}`, () => api.packages.yank(props.name, version, true, reason))
}

const unyank = (version: string) => run(`已恢复 ${version}`, () => api.packages.yank(props.name, version, false, ''))

function removeRelease(version: string) {
  if (!confirm(`删除版本 ${version} 及其文件？不可恢复。`)) return
  return run(`已删除 ${version}`, () => api.packages.removeRelease(props.name, version))
}

async function removePackage() {
  if (!pkg.value || !confirm(`删除整个包 ${pkg.value.name}（含所有版本、文件、授权）？不可恢复。`)) return
  try {
    await api.packages.remove(props.name)
    notify.success('已删除')
    router.push(admin('/packages'))
  } catch (err) {
    notify.error('删除失败', errorMessage(err))
  }
}
</script>

<template>
  <div v-if="detail && pkg">
    <PageHeader :title="pkg.name" :description="pkg.summary">
      <UButton :to="admin('/packages')" label="返回" icon="i-lucide-arrow-left" color="neutral" variant="ghost" />
      <UButton label="删除包" icon="i-lucide-trash-2" color="error" variant="subtle" data-test="delete-package" @click="removePackage" />
    </PageHeader>

    <div class="grid gap-6 lg:grid-cols-3">
      <div class="lg:col-span-2 space-y-6">
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <h2 class="font-medium">版本</h2>
              <span class="text-sm text-muted">最新 <span class="font-mono text-highlighted">{{ detail.latest_version || '—' }}</span></span>
            </div>
          </template>
          <p v-if="detail.releases.length === 0" class="text-sm text-muted">还没有任何版本。</p>
          <div v-for="r in detail.releases" :key="r.id" class="py-3 border-b border-default last:border-0">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-mono font-medium" :class="r.yanked ? 'line-through text-muted' : ''">{{ r.version }}</span>
              <UBadge v-if="r.yanked" color="warning" variant="subtle" size="sm">yanked{{ r.yanked_reason ? `：${r.yanked_reason}` : '' }}</UBadge>
              <span class="text-xs text-muted">{{ formatDate(r.created_at) }}</span>
              <span class="flex-1" />
              <UButton v-if="r.yanked" size="xs" variant="ghost" color="neutral" label="恢复" :data-test="`unyank-${r.version}`" :loading="busy" @click="unyank(r.version)" />
              <UButton v-else size="xs" variant="ghost" color="warning" label="yank" :data-test="`yank-${r.version}`" :loading="busy" @click="yank(r.version)" />
              <UButton size="xs" variant="ghost" color="error" icon="i-lucide-trash-2" :data-test="`delete-${r.version}`" :loading="busy" @click="removeRelease(r.version)" />
            </div>
            <ul class="mt-2 space-y-1">
              <li v-for="f in r.files" :key="f.id" class="text-sm flex flex-wrap items-center gap-x-3 gap-y-1">
                <span class="font-mono break-all">{{ f.filename }}</span>
                <span class="text-xs text-muted">{{ formatBytes(f.size) }}</span>
                <span v-if="f.requires_python" class="text-xs text-muted">python {{ f.requires_python }}</span>
                <span v-if="f.metadata_sha256" class="text-xs text-muted" title="PEP 658 元数据可用">metadata ✓</span>
                <span class="text-xs text-muted font-mono" :title="f.sha256">sha256 {{ f.sha256.slice(0, 12) }}…</span>
              </li>
              <li v-if="r.files.length === 0" class="text-xs text-muted">（没有文件）</li>
            </ul>
          </div>
        </UCard>

        <UCard v-if="pkg.description">
          <template #header><h2 class="font-medium">README</h2></template>
          <!-- Rendered as text on purpose: package metadata is admin-controlled but we still don't inject HTML. -->
          <pre class="text-sm whitespace-pre-wrap font-sans leading-relaxed">{{ pkg.description }}</pre>
        </UCard>
      </div>

      <div class="space-y-6">
        <UCard>
          <template #header><h2 class="font-medium">授权用户</h2></template>
          <p v-if="detail.users.length === 0" class="text-sm text-muted">还没有用户被授权。去 <RouterLink :to="admin('/users')" class="text-primary hover:underline">用户</RouterLink> 页授权。</p>
          <ul v-else class="divide-y divide-default text-sm">
            <li v-for="u in detail.users" :key="u.user_id" class="py-2 flex items-center justify-between gap-2">
              <RouterLink :to="admin(`/users/${u.user_id}`)" class="font-medium text-primary hover:underline">{{ u.username }}</RouterLink>
              <UBadge :color="expiryLabel(u.expires_at, u.active).color" variant="subtle" size="sm">{{ expiryLabel(u.expires_at, u.active).text }}</UBadge>
            </li>
          </ul>
        </UCard>

        <UCard>
          <template #header><h2 class="font-medium">安装</h2></template>
          <InstallSnippets :index-url="indexUrl || '/simple/'" :package-name="pkg.normalized_name" />
        </UCard>
      </div>
    </div>
  </div>
</template>
