<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ text: string; label?: string }>()
const copied = ref(false)

async function copy() {
  try {
    await navigator.clipboard.writeText(props.text)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    // Clipboard unavailable (http origin, old browser): user can still select the text.
  }
}
</script>

<template>
  <UButton
    size="xs"
    color="neutral"
    variant="ghost"
    :icon="copied ? 'i-lucide-check' : 'i-lucide-copy'"
    :label="copied ? '已复制' : (label ?? '复制')"
    @click="copy"
  />
</template>
