<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { Cloud, KeyRound, LockKeyhole, UserRound } from '@lucide/vue'
import { api } from '@/api'
import { loginWithPasskey } from '@/webauthn'
import { useAppStore, type Session } from '@/stores/app'

const username = ref('admin')
const password = ref('')
const loading = ref(false)
const passkeyLoading = ref(false)
const router = useRouter()
const route = useRoute()
const message = useMessage()
const app = useAppStore()
const finish = async (session: Session) => { app.setSession(session); await app.loadZones(); router.replace(String(route.query.redirect || '/dns')) }
const login = async () => { loading.value=true; try { await finish(await api.request<Session>('/auth/login',{method:'POST',body:JSON.stringify({username:username.value,password:password.value})})) } catch(error:any){message.error(error.message)} finally{loading.value=false} }
const passkeyLogin = async () => { passkeyLoading.value=true; try { await finish(await loginWithPasskey()) } catch(error:any){if(error?.name!=='NotAllowedError')message.error(error.message||'Passkey 登录失败')} finally{passkeyLoading.value=false} }
</script>

<template>
  <div class="flex min-h-screen bg-[#f4f5f7] dark:bg-[#111]">
    <section class="hidden w-[42%] flex-col justify-between bg-[#202326] p-12 text-white lg:flex">
      <div class="flex items-center gap-3"><div class="grid h-10 w-10 place-items-center rounded-md bg-[#f6821f]"><Cloud :size="22"/></div><span class="text-xl font-bold">FlareDNS</span></div>
      <div class="max-w-md"><p class="mb-5 text-xs font-semibold text-[#f6821f]">PRIVATE DNS CONTROL</p><h1 class="text-4xl font-bold leading-tight">更快地管理你的<br>Cloudflare DNS</h1><p class="mt-5 text-base leading-7 text-[#b8bec4]">专注于域名记录、代理状态和变更审计的自托管控制台。</p></div>
      <div class="text-xs text-[#8e959b]">数据保留在你的服务器中</div>
    </section>
    <main class="flex flex-1 items-center justify-center p-5">
      <div class="w-full max-w-[420px]">
        <div class="mb-8 flex items-center gap-3 lg:hidden"><div class="grid h-9 w-9 place-items-center rounded-md bg-[#f6821f] text-white"><Cloud :size="20"/></div><span class="text-xl font-bold">FlareDNS</span></div>
        <h2 class="text-2xl font-bold">登录控制台</h2><p class="mt-2 text-sm text-gray-500">使用管理员账户或已注册的 Passkey</p>
        <n-button block size="large" class="mt-7" :loading="passkeyLoading" @click="passkeyLogin"><template #icon><KeyRound :size="18"/></template>使用 Passkey 登录</n-button>
        <div class="my-6 flex items-center gap-3 text-xs text-gray-400"><span class="h-px flex-1 bg-gray-200 dark:bg-gray-700"></span>或使用密码<span class="h-px flex-1 bg-gray-200 dark:bg-gray-700"></span></div>
        <n-form @submit.prevent="login">
          <n-form-item label="用户名"><n-input v-model:value="username" size="large" autocomplete="username"><template #prefix><UserRound :size="17"/></template></n-input></n-form-item>
          <n-form-item label="密码"><n-input v-model:value="password" type="password" show-password-on="click" size="large" autocomplete="current-password" @keyup.enter="login"><template #prefix><LockKeyhole :size="17"/></template></n-input></n-form-item>
          <n-button type="primary" block size="large" :loading="loading" @click="login">登录</n-button>
        </n-form>
      </div>
    </main>
  </div>
</template>
