<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuth } from '../auth'
import { admin } from '../config'

const auth = useAuth()
const router = useRouter()

const nav = [
  { label: '包', to: admin('/packages'), icon: 'i-lucide-package' },
  { label: '用户', to: admin('/users'), icon: 'i-lucide-users' },
  { label: '上传', to: admin('/upload'), icon: 'i-lucide-upload' },
  { label: '下载记录', to: admin('/downloads'), icon: 'i-lucide-download' },
]

async function logout() {
  await auth.logout()
  router.push(admin('/login'))
}
</script>

<template>
  <div class="min-h-screen bg-default text-default">
    <header class="border-b border-default bg-elevated/40 backdrop-blur sticky top-0 z-10">
      <div class="max-w-6xl mx-auto px-4 h-14 flex items-center gap-2">
        <RouterLink :to="admin('/packages')" class="flex items-center gap-2 font-semibold text-highlighted mr-4">
          <UIcon name="i-lucide-box" class="size-5 text-primary" />
          johnnybt-pypi
        </RouterLink>
        <nav class="flex items-center gap-1 flex-1 overflow-x-auto">
          <UButton
            v-for="item in nav"
            :key="item.to"
            :to="item.to"
            :label="item.label"
            :icon="item.icon"
            color="neutral"
            variant="ghost"
            active-color="primary"
            active-variant="subtle"
            size="sm"
          />
        </nav>
        <UButton to="/" label="客户自查页" icon="i-lucide-external-link" color="neutral" variant="ghost" size="sm" />
        <UButton :to="admin('/settings')" icon="i-lucide-settings" color="neutral" variant="ghost" size="sm" :label="auth.user.value?.username" />
        <UButton icon="i-lucide-log-out" color="neutral" variant="ghost" size="sm" aria-label="退出登录" @click="logout" />
      </div>
    </header>
    <main class="max-w-6xl mx-auto px-4 py-8">
      <slot />
    </main>
  </div>
</template>
