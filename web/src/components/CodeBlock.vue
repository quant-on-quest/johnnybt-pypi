<script setup lang="ts">
import CopyButton from './CopyButton.vue'

defineProps<{ code: string; title?: string }>()
</script>

<template>
  <div class="rounded-lg border border-default bg-elevated/50 overflow-hidden">
    <div v-if="title" class="flex items-center justify-between px-3 py-1.5 border-b border-default text-xs text-muted">
      <span class="font-medium">{{ title }}</span>
      <CopyButton :text="code" />
    </div>
    <pre class="px-3 py-2 text-xs leading-relaxed overflow-x-auto font-mono whitespace-pre">{{ code }}</pre>
  </div>
</template>
