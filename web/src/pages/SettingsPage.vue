<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type ServerConfig } from '../api'
import { errorMessage, useNotify } from '../notify'
import PageHeader from '../components/PageHeader.vue'
import CodeBlock from '../components/CodeBlock.vue'

const notify = useNotify()
const config = ref<ServerConfig | null>(null)
const form = reactive({ current: '', next: '', confirm: '' })
const error = ref('')
const saving = ref(false)

onMounted(async () => {
  try {
    config.value = await api.config()
  } catch (err) {
    notify.error('加载配置失败', errorMessage(err))
  }
})

async function onSubmit(e: { data: { current: string; next: string; confirm: string } }) {
  error.value = ''
  if (e.data.next !== e.data.confirm) {
    error.value = '两次输入的新密码不一致'
    return
  }
  saving.value = true
  try {
    await api.auth.changePassword(e.data.current, e.data.next)
    notify.success('密码已修改')
    form.current = form.next = form.confirm = ''
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="设置" />
    <div class="grid gap-6 lg:grid-cols-2">
      <UCard>
        <template #header><h2 class="font-medium">修改管理员密码</h2></template>
        <UForm :state="form" class="space-y-3" @submit="onSubmit">
          <UFormField label="当前密码" name="current">
            <UInput v-model="form.current" type="password" autocomplete="current-password" class="w-full" />
          </UFormField>
          <UFormField label="新密码" name="next" hint="至少 8 位">
            <UInput v-model="form.next" type="password" autocomplete="new-password" class="w-full" />
          </UFormField>
          <UFormField label="再输一次" name="confirm">
            <UInput v-model="form.confirm" type="password" autocomplete="new-password" class="w-full" />
          </UFormField>
          <p v-if="error" class="text-sm text-error">{{ error }}</p>
          <UButton type="submit" label="修改密码" :loading="saving" />
        </UForm>
        <p class="text-xs text-muted mt-4">忘记密码时在服务器上执行 <code>pypi-server admin reset-password</code>，会生成新密码并让现有登录失效。</p>
      </UCard>

      <UCard v-if="config">
        <template #header><h2 class="font-medium">服务地址</h2></template>
        <div class="space-y-3">
          <CodeBlock title="索引地址（给客户）" :code="config.index_url" />
          <CodeBlock title="上传地址（uv publish / twine）" :code="config.upload_url" />
        </div>
        <p class="text-xs text-muted mt-4">
          地址来自 <code>PYPI_BASE_URL</code>（或启用 TLS 时的第一个域名）。反向代理后面请设置它，否则安装片段里的 host 可能不对。
        </p>
      </UCard>
    </div>
  </div>
</template>
