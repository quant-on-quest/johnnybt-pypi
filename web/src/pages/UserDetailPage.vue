<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, type PackageSummary, type TokenCreated, type UserDetail } from '../api'
import { errorMessage, useNotify } from '../notify'
import { expiryLabel, formatDate, formatDateTime } from '../format'
import PageHeader from '../components/PageHeader.vue'
import { admin } from '../config'
import InstallSnippets from '../components/InstallSnippets.vue'
import CopyButton from '../components/CopyButton.vue'

const props = defineProps<{ id: number }>()
const notify = useNotify()
const router = useRouter()

const detail = ref<UserDetail | null>(null)
const packages = ref<PackageSummary[]>([])
const indexUrl = ref('')
const edit = reactive({ username: '', note: '' })
const grant = reactive({ package: '', expires_at: '' })
const tokenForm = reactive({ name: '', scope: 'read' as 'read' | 'write' })
const created = ref<TokenCreated | null>(null)
const busy = ref(false)

const user = computed(() => detail.value?.user)
const packageOptions = computed(() => packages.value.map((p) => ({ label: p.name, value: p.normalized_name })))
const scopeOptions = computed(() =>
  user.value?.is_admin
    ? [
        { label: 'read（安装）', value: 'read' },
        { label: 'write（上传 / uv publish）', value: 'write' },
      ]
    : [{ label: 'read（安装）', value: 'read' }],
)
const snippetPackage = computed(() => detail.value?.entitlements[0]?.normalized_name ?? '')

async function load() {
  try {
    detail.value = await api.users.get(props.id)
    edit.username = detail.value.user.username
    edit.note = detail.value.user.note
  } catch (err) {
    notify.error('加载用户失败', errorMessage(err))
  }
}

onMounted(async () => {
  await load()
  try {
    const [pkgs, cfg] = await Promise.all([api.packages.list(), api.config()])
    packages.value = pkgs
    indexUrl.value = cfg.index_url
  } catch (err) {
    notify.error('加载失败', errorMessage(err))
  }
})
watch(() => props.id, load)

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

const saveProfile = (e: { data: { username: string; note: string } }) =>
  run('已保存', () => api.users.update(props.id, e.data.username, e.data.note))

const onGrant = (e: { data: { package: string; expires_at: string } }) =>
  run('已授权', async () => {
    await api.users.grant(props.id, e.data.package, e.data.expires_at)
    grant.package = ''
    grant.expires_at = ''
  })

const onRevoke = (pkg: string) => run('已取消授权', () => api.users.revoke(props.id, pkg))

const onCreateToken = (e: { data: { name: string; scope: 'read' | 'write' } }) =>
  run('已生成 token', async () => {
    created.value = await api.users.createToken(props.id, e.data.name, e.data.scope)
    tokenForm.name = ''
  })

function onRevokeToken(id: number, prefix: string) {
  if (!confirm(`吊销 token ${prefix}…？使用它的客户端会立刻失效。`)) return
  return run('已吊销', () => api.tokens.revoke(id))
}

async function onDelete() {
  if (!user.value || !confirm(`删除用户 ${user.value.username}？其授权和 token 一并删除。`)) return
  try {
    await api.users.remove(props.id)
    notify.success('已删除')
    router.push(admin('/users'))
  } catch (err) {
    notify.error('删除失败', errorMessage(err))
  }
}
</script>

<template>
  <div v-if="detail && user">
    <PageHeader :title="user.username" :description="user.is_admin ? '管理员' : `客户 · 创建于 ${formatDate(user.created_at)}`">
      <UButton :to="admin('/users')" label="返回" icon="i-lucide-arrow-left" color="neutral" variant="ghost" />
      <UButton v-if="!user.is_admin" label="删除用户" icon="i-lucide-trash-2" color="error" variant="subtle" @click="onDelete" />
    </PageHeader>

    <div class="grid gap-6 lg:grid-cols-3">
      <!-- profile -->
      <UCard>
        <template #header><h2 class="font-medium">资料</h2></template>
        <UForm :state="edit" class="space-y-3" data-test="profile-form" @submit="saveProfile">
          <UFormField label="用户名" name="username" hint="GitHub 名或微信名都行，只用于你自己辨认">
            <UInput v-model="edit.username" class="w-full" />
          </UFormField>
          <UFormField label="备注" name="note">
            <UTextarea v-model="edit.note" :rows="3" class="w-full" />
          </UFormField>
          <UButton type="submit" label="保存" size="sm" :loading="busy" />
        </UForm>
      </UCard>

      <!-- entitlements -->
      <UCard class="lg:col-span-2">
        <template #header><h2 class="font-medium">授权的包</h2></template>
        <UForm :state="grant" class="flex flex-wrap items-end gap-2 mb-4" data-test="grant-form" @submit="onGrant">
          <UFormField label="包" name="package" class="min-w-48 flex-1">
            <USelectMenu v-model="grant.package" :items="packageOptions" value-key="value" placeholder="选择包" class="w-full" />
          </UFormField>
          <UFormField label="到期日" name="expires_at" hint="留空 = 永久">
            <UInput v-model="grant.expires_at" type="date" />
          </UFormField>
          <UButton type="submit" label="授权" icon="i-lucide-plus" :disabled="!grant.package" :loading="busy" />
        </UForm>

        <p v-if="detail.entitlements.length === 0" class="text-sm text-muted">还没有授权任何包。</p>
        <table v-else class="w-full text-sm">
          <tbody class="divide-y divide-default">
            <tr v-for="e in detail.entitlements" :key="e.package_id">
              <td class="py-2">
                <RouterLink :to="admin(`/packages/${e.normalized_name}`)" class="font-medium text-primary hover:underline">{{ e.package_name }}</RouterLink>
                <span class="text-xs text-muted ml-2 font-mono">{{ e.latest_version || '' }}</span>
              </td>
              <td class="py-2">
                <UBadge :color="expiryLabel(e.expires_at, e.active).color" variant="subtle" size="sm">{{ expiryLabel(e.expires_at, e.active).text }}</UBadge>
              </td>
              <td class="py-2 text-xs text-muted">授权于 {{ formatDate(e.granted_at) }}</td>
              <td class="py-2 text-right">
                <UButton size="xs" color="error" variant="ghost" icon="i-lucide-x" label="取消" :data-test="`revoke-${e.normalized_name}`" @click="onRevoke(e.normalized_name)" />
              </td>
            </tr>
          </tbody>
        </table>
      </UCard>

      <!-- tokens -->
      <UCard class="lg:col-span-3">
        <template #header><h2 class="font-medium">API token</h2></template>

        <UAlert
          v-if="created"
          color="success"
          variant="subtle"
          icon="i-lucide-key"
          title="新 token 只显示这一次，发给客户后关掉此页即可"
          class="mb-4"
        >
          <template #description>
            <div class="flex items-center gap-2 font-mono text-sm break-all mt-1">
              <span data-test="new-token">{{ created.token }}</span>
              <CopyButton :text="created.token" />
            </div>
            <div class="mt-4">
              <InstallSnippets :index-url="created.index_url" :token="created.token" :package-name="snippetPackage" />
            </div>
          </template>
        </UAlert>

        <UForm :state="tokenForm" class="flex flex-wrap items-end gap-2 mb-4" data-test="token-form" @submit="onCreateToken">
          <UFormField label="名称" name="name" hint="例如 laptop / CI">
            <UInput v-model="tokenForm.name" placeholder="可选" />
          </UFormField>
          <UFormField label="权限" name="scope">
            <USelect v-model="tokenForm.scope" :items="scopeOptions" value-key="value" class="w-56" />
          </UFormField>
          <UButton type="submit" label="生成 token" icon="i-lucide-key" :loading="busy" />
        </UForm>

        <p v-if="detail.tokens.length === 0" class="text-sm text-muted">还没有 token。</p>
        <table v-else class="w-full text-sm">
          <thead class="text-left text-xs uppercase text-muted">
            <tr>
              <th class="py-1 font-medium">前缀</th>
              <th class="py-1 font-medium">名称</th>
              <th class="py-1 font-medium">权限</th>
              <th class="py-1 font-medium">创建</th>
              <th class="py-1 font-medium">最近使用</th>
              <th class="py-1 font-medium">状态</th>
              <th></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-default">
            <tr v-for="t in detail.tokens" :key="t.id" :class="t.revoked_at ? 'opacity-50' : ''">
              <td class="py-2 font-mono">{{ t.prefix }}…</td>
              <td class="py-2">{{ t.name || '—' }}</td>
              <td class="py-2"><UBadge :color="t.scope === 'write' ? 'warning' : 'neutral'" variant="subtle" size="sm">{{ t.scope }}</UBadge></td>
              <td class="py-2 text-muted">{{ formatDate(t.created_at) }}</td>
              <td class="py-2 text-muted">{{ formatDateTime(t.last_used_at) }}</td>
              <td class="py-2">
                <UBadge v-if="t.revoked_at" color="error" variant="subtle" size="sm">已吊销 {{ formatDate(t.revoked_at) }}</UBadge>
                <UBadge v-else color="success" variant="subtle" size="sm">有效</UBadge>
              </td>
              <td class="py-2 text-right">
                <UButton v-if="!t.revoked_at" size="xs" color="error" variant="ghost" label="吊销" :data-test="`revoke-token-${t.id}`" @click="onRevokeToken(t.id, t.prefix)" />
              </td>
            </tr>
          </tbody>
        </table>
      </UCard>
    </div>
  </div>
</template>
