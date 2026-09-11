<script lang="ts">
export interface SnippetInput {
  indexUrl: string
  token?: string
  packageName?: string
  /** Name used for the uv index entry; defaults to "johnnybt". */
  indexName?: string
}

export interface Snippets {
  uvCli: string
  pipCli: string
  uvToml: string
  uvEnv: string
  pipConf: string
}

/** Pure builder so the exact text can be unit-tested. */
export function buildSnippets(input: SnippetInput): Snippets {
  const token = input.token || '<你的token>'
  const pkg = input.packageName || '<package>'
  const indexName = input.indexName || 'johnnybt'
  const envName = indexName.toUpperCase().replace(/[^A-Z0-9]/g, '_')
  const withCreds = input.indexUrl.replace(/^(https?:\/\/)/, `$1__token__:${token}@`)
  return {
    uvCli: `uv pip install --index-url ${withCreds} ${pkg}`,
    pipCli: `pip install --index-url ${withCreds} ${pkg}`,
    uvToml: [
      '# uv.toml 或 pyproject.toml 的 [tool.uv] 段',
      '[[index]]',
      `name = "${indexName}"`,
      `url = "${input.indexUrl}"`,
    ].join('\n'),
    uvEnv: [
      '# 凭证放环境变量，不进配置文件和 shell 历史',
      `export UV_INDEX_${envName}_USERNAME=__token__`,
      `export UV_INDEX_${envName}_PASSWORD=${token}`,
    ].join('\n'),
    pipConf: [
      '# ~/.pip/pip.conf（Windows: %APPDATA%\\pip\\pip.ini）',
      '[global]',
      `extra-index-url = ${withCreds}`,
    ].join('\n'),
  }
}
</script>

<script setup lang="ts">
import { computed } from 'vue'
import CodeBlock from './CodeBlock.vue'

const props = defineProps<SnippetInput>()
const s = computed(() => buildSnippets(props))
</script>

<template>
  <div class="space-y-3">
    <CodeBlock title="一次性安装（uv）" :code="s.uvCli" />
    <CodeBlock title="一次性安装（pip）" :code="s.pipCli" />
    <div class="grid gap-3 md:grid-cols-2">
      <CodeBlock title="长期配置：uv 索引" :code="s.uvToml" />
      <CodeBlock title="长期配置：uv 凭证" :code="s.uvEnv" />
    </div>
    <CodeBlock title="长期配置：pip" :code="s.pipConf" />
  </div>
</template>
