<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type ServerConfig, type UploadResult } from '../api'
import { errorMessage, useNotify } from '../notify'
import PageHeader from '../components/PageHeader.vue'
import { admin } from '../config'
import CodeBlock from '../components/CodeBlock.vue'

const notify = useNotify()
const files = ref<File[]>([])
const results = ref<UploadResult[]>([])
const uploading = ref(false)
const config = ref<ServerConfig | null>(null)

onMounted(async () => {
  try {
    config.value = await api.config()
  } catch (err) {
    notify.error('加载配置失败', errorMessage(err))
  }
})

function onFiles(value: File | File[] | null | undefined) {
  files.value = Array.isArray(value) ? value : value ? [value] : []
}

async function upload() {
  if (files.value.length === 0) return
  uploading.value = true
  try {
    results.value = await api.packages.upload(files.value)
    const ok = results.value.filter((r) => !r.error).length
    if (ok > 0) notify.success(`已上传 ${ok} 个文件`)
    files.value = []
  } catch (err) {
    notify.error('上传失败', errorMessage(err))
  } finally {
    uploading.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="上传" description="拖入 .whl / .tar.gz，服务端会解析 METADATA 自动建包和版本。" />

    <div class="grid gap-6 lg:grid-cols-2">
      <UCard>
        <template #header><h2 class="font-medium">从浏览器上传</h2></template>
        <UFileUpload
          :model-value="files"
          multiple
          accept=".whl,.tar.gz,.zip"
          label="拖入或点击选择分发文件"
          description="wheel (.whl) 或 sdist (.tar.gz / .zip)"
          class="min-h-40"
          @update:model-value="onFiles"
        />
        <div class="flex items-center justify-between mt-4">
          <span class="text-sm text-muted">{{ files.length }} 个文件待上传</span>
          <UButton label="上传" icon="i-lucide-upload" :disabled="files.length === 0" :loading="uploading" data-test="upload-button" @click="upload" />
        </div>

        <ul v-if="results.length" class="mt-4 space-y-1 text-sm">
          <li v-for="r in results" :key="r.filename" class="flex items-start gap-2">
            <UIcon :name="r.error ? 'i-lucide-x-circle' : 'i-lucide-check-circle'" :class="r.error ? 'text-error' : 'text-success'" class="size-4 mt-0.5 shrink-0" />
            <span>
              <span class="font-mono">{{ r.filename }}</span>
              <span v-if="r.error" class="text-error"> — {{ r.error }}</span>
              <span v-else class="text-muted"> → <RouterLink :to="admin(`/packages/${r.package}`)" class="text-primary hover:underline">{{ r.package }} {{ r.version }}</RouterLink></span>
            </span>
          </li>
        </ul>
      </UCard>

      <UCard>
        <template #header><h2 class="font-medium">从命令行发布</h2></template>
        <p class="text-sm text-muted mb-3">
          先在 <RouterLink :to="admin('/users')" class="text-primary hover:underline">用户</RouterLink> 页给管理员账号生成一个 <code>write</code> token，然后：
        </p>
        <div v-if="config" class="space-y-3">
          <CodeBlock title="uv" :code="`uv build\nuv publish --publish-url ${config.upload_url} --username __token__ --password <write-token> dist/*`" />
          <CodeBlock title="twine" :code="`twine upload --repository-url ${config.upload_url} -u __token__ -p <write-token> dist/*`" />
          <CodeBlock title="~/.pypirc（免密）" :code="`[distutils]\nindex-servers = johnnybt\n\n[johnnybt]\nrepository = ${config.upload_url}\nusername = __token__\npassword = <write-token>`" />
        </div>
        <p class="text-xs text-muted mt-3">重复上传同一文件名会被拒绝（twine 的 <code>--skip-existing</code> 能识别）。要覆盖请先在包页面删除该版本。</p>
      </UCard>
    </div>
  </div>
</template>
