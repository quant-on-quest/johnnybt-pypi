<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuth } from '../auth'
import { errorMessage } from '../notify'
import { admin } from '../config'

const auth = useAuth()
const router = useRouter()
const route = useRoute()

const state = reactive({ username: 'admin', password: '' })
const error = ref('')
const loading = ref(false)

async function onSubmit(event: { data: { username: string; password: string } }) {
  error.value = ''
  loading.value = true
  try {
    await auth.login(event.data.username, event.data.password)
    const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') ? route.query.next : admin('/packages')
    await router.push(next)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-default px-4">
    <UCard class="w-full max-w-sm">
      <div class="flex items-center gap-2 mb-6">
        <UIcon name="i-lucide-box" class="size-6 text-primary" />
        <div>
          <h1 class="font-semibold text-highlighted">johnnybt-pypi</h1>
          <p class="text-xs text-muted">管理员登录</p>
        </div>
      </div>
      <UForm :state="state" class="space-y-4" @submit="onSubmit">
        <UFormField label="用户名" name="username">
          <UInput v-model="state.username" autocomplete="username" class="w-full" />
        </UFormField>
        <UFormField label="密码" name="password">
          <UInput v-model="state.password" type="password" autocomplete="current-password" class="w-full" />
        </UFormField>
        <p v-if="error" class="text-sm text-error">{{ error }}</p>
        <UButton type="submit" label="登录" block :loading="loading" />
      </UForm>
      <p class="text-xs text-muted mt-6">
        首次启动的密码在服务器终端和 <code>data/initial_admin_password</code> 里；忘记了可执行
        <code>pypi-server admin reset-password</code>。
      </p>
    </UCard>
  </div>
</template>
