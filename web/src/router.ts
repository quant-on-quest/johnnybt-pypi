import { createMemoryHistory, createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuth } from './auth'
import { admin } from './config'

/**
 * Public surface is the customer self-check page at "/". The admin UI lives
 * under the prefix the server injected (PYPI_ADMIN_PATH), so routes are built
 * at creation time rather than declared statically.
 */
function buildRoutes(): RouteRecordRaw[] {
  return [
    { path: '/', name: 'me', component: () => import('./pages/MePage.vue'), meta: { public: true } },
    { path: '/me', redirect: '/' },
    { path: admin(''), redirect: admin('/packages') },
    { path: admin('/login'), name: 'login', component: () => import('./pages/LoginPage.vue'), meta: { public: true, guestOnly: true } },
    { path: admin('/packages'), name: 'packages', component: () => import('./pages/PackagesPage.vue') },
    { path: admin('/packages/:name'), name: 'package', component: () => import('./pages/PackageDetailPage.vue'), props: true },
    { path: admin('/upload'), name: 'upload', component: () => import('./pages/UploadPage.vue') },
    { path: admin('/users'), name: 'users', component: () => import('./pages/UsersPage.vue') },
    { path: admin('/users/:id'), name: 'user', component: () => import('./pages/UserDetailPage.vue'), props: (r) => ({ id: Number(r.params.id) }) },
    { path: admin('/downloads'), name: 'downloads', component: () => import('./pages/DownloadsPage.vue') },
    { path: admin('/settings'), name: 'settings', component: () => import('./pages/SettingsPage.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ]
}

export function createAppRouter(memory = false) {
  const router = createRouter({
    history: memory ? createMemoryHistory() : createWebHistory(),
    routes: buildRoutes(),
  })

  router.beforeEach(async (to) => {
    const auth = useAuth()
    if (to.meta.public && !to.meta.guestOnly) return true
    await auth.ensure()
    if (to.meta.guestOnly) {
      return auth.isLoggedIn.value ? admin('/packages') : true
    }
    if (!auth.isLoggedIn.value) {
      return { path: admin('/login'), query: { next: to.fullPath } }
    }
    return true
  })

  return router
}
