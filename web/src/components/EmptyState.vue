<script setup lang="ts">
defineProps<{ icon?: string; title: string; description?: string }>()
</script>

<template>
  <div class="rounded-lg border border-dashed border-default p-10 text-center">
    <UIcon :name="icon ?? 'i-lucide-inbox'" class="size-8 text-muted mx-auto mb-3" />
    <p class="font-medium text-highlighted">{{ title }}</p>
    <p v-if="description" class="text-sm text-muted mt-1">{{ description }}</p>
    <div class="mt-4"><slot /></div>
  </div>
</template>
