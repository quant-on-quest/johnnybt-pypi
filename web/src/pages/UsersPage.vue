<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, type UserSummary } from '../api'
import { errorMessage, useNotify } from '../notify'
import { formatDate } from '../format'
import PageHeader from '../components/PageHeader.vue'
import { admin } from '../config'

const notify = useNotify()
const router = useRouter()
const users = ref<UserSummary[]>([])
const search = ref('')
const form = reactive({ username: '', note: '' })
const creating = ref(false)

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return users.value
  return users.value.filter((u) => u.username.toLowerCase().includes(q) || u.note.toLowerCase().includes(q))
})

async function load() {
  try {
    users.value = await api.users.list()
  } catch (err) {
    notify.error('加载用户失败', errorMessage(err))
  }
}
onMounted(load)

async function onCreate(e: { data: { username: string; note: string } }) {
  creating.value = true
  try {
    const u = await api.users.create(e.data.username.trim(), e.data.note.trim())
    notify.success(`已创建 ${u.username}`, '接下来给他授权并生成 token')
    form.username = ''
    form.note = ''
    router.push(admin(`/users/${u.id}`))
  } catch (err) {
    notify.error('创建失败', errorMessage(err))
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="用户" description="客户不需要注册：你在这里建账号、授权、发 token。">
      <UInput v-model="search" icon="i-lucide-search" placeholder="搜索用户名 / 备注" class="w-56" />
    </PageHeader>

    <UCard class="mb-6">
      <UForm :state="form" class="flex flex-wrap items-end gap-2" @submit="onCreate">
        <UFormField label="用户名" name="username" hint="GitHub 名 / 微信名，只要你认得出">
          <UInput v-model="form.username" placeholder="alice" />
        </UFormField>
        <UFormField label="备注" name="note" class="flex-1 min-w-48">
          <UInput v-model="form.note" placeholder="可选：公司、联系方式、付款信息…" class="w-full" />
        </UFormField>
        <UButton type="submit" label="新建用户" icon="i-lucide-user-plus" :disabled="!form.username.trim()" :loading="creating" />
      </UForm>
    </UCard>

    <div class="rounded-lg border border-default overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-elevated/50 text-left text-xs uppercase text-muted">
          <tr>
            <th class="px-4 py-2 font-medium">用户</th>
            <th class="px-4 py-2 font-medium">备注</th>
            <th class="px-4 py-2 font-medium text-right">授权包</th>
            <th class="px-4 py-2 font-medium text-right">有效 token</th>
            <th class="px-4 py-2 font-medium">创建于</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-default">
          <tr v-for="u in filtered" :key="u.id" class="hover:bg-elevated/30">
            <td class="px-4 py-2">
              <RouterLink :to="admin(`/users/${u.id}`)" class="font-medium text-primary hover:underline">{{ u.username }}</RouterLink>
              <UBadge v-if="u.is_admin" color="warning" variant="subtle" size="sm" class="ml-2">admin</UBadge>
            </td>
            <td class="px-4 py-2 text-muted truncate max-w-md">{{ u.note }}</td>
            <td class="px-4 py-2 text-right tabular-nums">{{ u.entitlement_count }}</td>
            <td class="px-4 py-2 text-right tabular-nums">{{ u.token_count }}</td>
            <td class="px-4 py-2 text-muted">{{ formatDate(u.created_at) }}</td>
          </tr>
          <tr v-if="filtered.length === 0">
            <td colspan="5" class="px-4 py-6 text-center text-muted">没有匹配的用户</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
