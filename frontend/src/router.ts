import { createRouter, createWebHistory } from 'vue-router'
import { useAppStore } from '@/stores/app'

export const router = createRouter({ history: createWebHistory(), routes: [
  { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
  { path: '/', redirect: '/dns' },
  { path: '/dns', name: 'dns', component: () => import('@/views/DNSView.vue') },
  { path: '/domains', name: 'domains', component: () => import('@/views/DomainsView.vue') },
  { path: '/tokens', name: 'tokens', component: () => import('@/views/TokensView.vue') },
  { path: '/logs', name: 'logs', component: () => import('@/views/LogsView.vue') },
  { path: '/settings', name: 'settings', component: () => import('@/views/SettingsView.vue') },
  { path: '/:pathMatch(.*)*', redirect: '/dns' },
] })

router.beforeEach(to => { const app = useAppStore(); if (!to.meta.public && !app.authenticated) return { name: 'login', query: { redirect: to.fullPath } }; if (to.name === 'login' && app.authenticated) return { name: 'dns' } })
