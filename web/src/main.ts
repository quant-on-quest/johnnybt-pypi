import './assets/css/main.css'

import { createApp } from 'vue'
import ui from '@nuxt/ui/vue-plugin'
import App from './App.vue'
import { createAppRouter } from './router'

const app = createApp(App)
app.use(createAppRouter())
app.use(ui)
app.mount('#app')
