<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppShell from './components/AppShell.vue'

const route = useRoute()
// Login and the customer self-check page stand alone; everything else is admin chrome.
const bare = computed(() => route.meta.public === true)
</script>

<template>
  <UApp :toaster="{ position: 'top-right' }">
    <RouterView v-if="bare" />
    <AppShell v-else>
      <RouterView />
    </AppShell>
  </UApp>
</template>
