<script lang="ts">
export interface SnippetInput {
  indexUrl: string
  token?: string
  packageName?: string
  /** Name used for the uv index entry; defaults to "johnnybt". */
  indexName?: string
}

export interface Snippets {
  /** Step 1 — store the token in uv's credential store. */
  uvLogin: string
  /** Step 2 — user-level index config. */
  uvConfig: string
  /** Step 3 — everyday commands, no credentials involved. */
  uvUse: string
  /** Advanced — pin the package to this index inside one project. */
  projectPin: string
  /** Advanced — CI / containers without `uv auth`. */
  ciEnv: string
  /** Fallback — pip. */
  pipConf: string
  pipCli: string
}

/** Pure builder so the exact text can be unit-tested. */
export function buildSnippets(input: SnippetInput): Snippets {
  const token = input.token || '<你的token>'
  const pkg = input.packageName || '<package>'
  const indexName = input.indexName || 'johnnybt'
  const envName = indexName.toUpperCase().replace(/[^A-Z0-9]/g, '_')
  const withCreds = input.indexUrl.replace(/^(https?:\/\/)/, `$1__token__:${token}@`)
  return {
    uvLogin: `uv auth login ${input.indexUrl} --token ${token}`,
    uvConfig: [
      '# ~/.config/uv/uv.toml（macOS / Linux）  或  %APPDATA%\\uv\\uv.toml（Windows）',
      '[[index]]',
      `name = "${indexName}"`,
      `url = "${input.indexUrl}"`,
      'authenticate = "always"',
    ].join('\n'),
    uvUse: [
      `uv add ${pkg}            # 加进项目`,
      `uvx ${pkg}               # 直接运行包里的命令行工具`,
      `uv pip install ${pkg}    # 装进当前环境`,
    ].join('\n'),
    projectPin: [
      '# pyproject.toml —— 只在这个项目里启用私有索引，并把包钉死在它上面（防依赖混淆）',
      '[[tool.uv.index]]',
      `name = "${indexName}"`,
      `url = "${input.indexUrl}"`,
      'explicit = true',
      '',
      '[tool.uv.sources]',
      `${pkg} = { index = "${indexName}" }`,
    ].join('\n'),
    ciEnv: [
      '# CI / 容器里没法 uv auth login 时：凭证放环境变量（用 Secret 注入）',
      `export UV_INDEX_${envName}_USERNAME=__token__`,
      `export UV_INDEX_${envName}_PASSWORD=${token}`,
    ].join('\n'),
    pipConf: [
      '# ~/.pip/pip.conf（Windows: %APPDATA%\\pip\\pip.ini）',
      '[global]',
      `extra-index-url = ${withCreds}`,
    ].join('\n'),
    pipCli: `pip install --index-url ${withCreds} ${pkg}`,
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
    <p class="text-sm text-muted">
      需要 uv ≥ 0.8.18（旧版本执行 <code>uv self update</code>）。前两步只做一次，之后所有 uv 命令直接可用，token 不进命令行、不进 lock 文件。
    </p>
    <CodeBlock title="1. 登录（一次）" :code="s.uvLogin" />
    <CodeBlock title="2. 全局索引配置（一次）" :code="s.uvConfig" />
    <CodeBlock title="3. 之后正常用" :code="s.uvUse" />

    <details class="rounded-lg border border-default">
      <summary class="cursor-pointer select-none px-3 py-2 text-sm font-medium text-muted hover:text-highlighted">高级：项目级钉死 / CI / pip</summary>
      <div class="space-y-3 p-3 border-t border-default">
        <CodeBlock title="项目级：只在此项目启用并钉死索引" :code="s.projectPin" />
        <CodeBlock title="CI / 容器：环境变量" :code="s.ciEnv" />
        <CodeBlock title="pip：配置文件" :code="s.pipConf" />
        <CodeBlock title="pip：一次性" :code="s.pipCli" />
      </div>
    </details>
  </div>
</template>
