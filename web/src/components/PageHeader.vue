<script setup lang="ts">
defineProps<{ title: string; description?: string }>()
</script>

<template>
  <div class="flex flex-wrap items-end justify-between gap-3 mb-6">
    <div>
      <h1 class="text-2xl font-semibold text-highlighted">{{ title }}</h1>
      <p v-if="description" class="text-sm text-muted mt-1">{{ description }}</p>
    </div>
    <div class="flex items-center gap-2">
      <slot />
    </div>
  </div>
</template>
