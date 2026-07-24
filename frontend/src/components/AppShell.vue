<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { ChevronDown, Cloud, FileClock, KeyRound, LogOut, Menu, Moon, Network, RefreshCw, Settings, Sun, X } from '@lucide/vue'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import BrandMark from '@/components/BrandMark.vue'

const app = useAppStore()
const route = useRoute()
const router = useRouter()
const message = useMessage()
const mobileOpen = ref(false)
const refreshing = ref(false)
const icon = (component: any) => () => h(component, { size: 18, strokeWidth: 1.8 })
const menuOptions = [
  { label: 'DNS 记录', key: '/dns', icon: icon(Network) },
  { label: '域名', key: '/domains', icon: icon(Cloud) },
  { label: 'API Token', key: '/tokens', icon: icon(KeyRound) },
  { label: '操作日志', key: '/logs', icon: icon(FileClock) },
  { label: '设置', key: '/settings', icon: icon(Settings) },
]
const zoneOptions = computed(() => app.zones.map(zone => ({ label: `${zone.name} · ${zone.token_name}`, value: zone.id })))
const activeZone = computed({ get: () => Number(route.query.zone || app.defaultZone?.id || 0), set: value => router.push({ path: '/dns', query: { zone: value } }) })

onMounted(async () => { if (!app.zonesLoaded) { try { await app.loadZones() } catch {} } })
const navigate = (path: string) => { mobileOpen.value = false; router.push(path) }
const logout = async () => { try { await api.request('/auth/logout', { method: 'POST', body: '{}' }) } finally { app.clear(); router.push('/login') } }
const refreshZones = async () => {
  refreshing.value = true
  try { await app.loadZones(); message.success('域名列表已刷新') } catch (error: any) { message.error(error.message) } finally { refreshing.value = false }
}
</script>

<template>
  <div class="min-h-screen bg-[#f4f5f7] dark:bg-[#111]">
    <aside class="fixed inset-y-0 left-0 z-30 hidden w-[228px] border-r border-[#34383d] bg-[#202326] text-white lg:block">
      <div class="flex h-16 items-center gap-3 border-b border-white/10 px-5">
        <BrandMark class="h-8 w-8 shrink-0" />
        <div><div class="text-[17px] font-bold">FlareDNS</div><div class="text-[10px] text-[#aeb4ba]">CLOUDFLARE DNS</div></div>
      </div>
      <n-menu :value="route.path" :options="menuOptions" :indent="18" :root-indent="16" inverted class="mt-3" @update:value="navigate"/>
      <div class="absolute bottom-0 left-0 right-0 border-t border-white/10 p-3">
        <button class="flex h-10 w-full items-center gap-3 rounded px-3 text-left text-sm text-[#d9dde1] hover:bg-white/8" @click="logout"><LogOut :size="17"/>退出登录</button>
      </div>
    </aside>

    <n-drawer v-model:show="mobileOpen" placement="left" :width="280">
      <n-drawer-content :native-scrollbar="false" body-content-style="padding: 0">
        <div class="flex h-16 items-center justify-between border-b px-4"><div class="flex items-center gap-2 font-bold"><BrandMark class="h-8 w-8 shrink-0" />FlareDNS</div><n-button quaternary circle aria-label="关闭导航" @click="mobileOpen=false"><X :size="18"/></n-button></div>
        <n-menu :value="route.path" :options="menuOptions" class="py-3" @update:value="navigate"/>
      </n-drawer-content>
    </n-drawer>

    <div class="lg:pl-[228px]">
      <header class="sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-[#dfe2e5] bg-white px-3 dark:border-[#343434] dark:bg-[#191919] sm:px-5">
        <n-button quaternary circle class="mobile-only-nav" aria-label="打开导航" @click="mobileOpen=true"><Menu :size="20"/></n-button>
        <div class="min-w-0 flex-1 sm:max-w-[400px]">
          <n-select v-if="app.zones.length" v-model:value="activeZone" :options="zoneOptions" filterable placeholder="选择域名" size="small"/>
          <span v-else class="text-sm text-gray-500">尚未同步域名</span>
        </div>
        <n-tooltip><template #trigger><n-button quaternary circle :loading="refreshing" aria-label="刷新域名" @click="refreshZones"><RefreshCw :size="18"/></n-button></template>刷新域名</n-tooltip>
        <n-tooltip><template #trigger><n-button quaternary circle aria-label="切换主题" @click="app.toggleTheme"><Sun v-if="app.dark" :size="18"/><Moon v-else :size="18"/></n-button></template>切换主题</n-tooltip>
        <n-dropdown :options="[{label:'设置',key:'settings',icon:icon(Settings)},{label:'退出登录',key:'logout',icon:icon(LogOut)}]" @select="(key:string)=>key==='logout'?logout():router.push('/settings')">
          <n-button text><span class="hidden text-sm sm:inline">{{ app.session?.username }}</span><ChevronDown :size="15"/></n-button>
        </n-dropdown>
      </header>
      <main class="mx-auto max-w-[1600px] p-3 sm:p-5 lg:p-6"><router-view/></main>
    </div>
  </div>
</template>
