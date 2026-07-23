<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NTag, type DataTableColumns, useMessage } from 'naive-ui'
import { ArrowRight, Star } from '@lucide/vue'
import { api } from '@/api'
import { useAppStore, type Zone } from '@/stores/app'
const app=useAppStore(), router=useRouter(), message=useMessage(), loading=ref(false)
const setDefault=async(zone:Zone)=>{try{await api.request(`/zones/${zone.id}/default`,{method:'PUT',body:'{}'});await app.loadZones();message.success(`已将 ${zone.name} 设为默认域名`)}catch(e:any){message.error(e.message)}}
const columns:DataTableColumns<Zone>=[{title:'域名',key:'name',render:r=>h('div',[h('div',{class:'font-semibold'},r.name),h('div',{class:'text-xs text-gray-500 mt-0.5'},r.token_name)])},{title:'状态',key:'status',width:110,render:r=>h(NTag,{type:r.status==='active'?'success':'warning',size:'small',bordered:false},{default:()=>r.status||'unknown'})},{title:'DNS 记录',key:'record_count',width:110},{title:'最后同步',key:'last_synced_at',width:180,render:r=>r.last_synced_at?new Date(r.last_synced_at).toLocaleString():'-'},{title:'操作',key:'actions',width:190,render:r=>h('div',{class:'flex gap-1'},[h(NButton,{text:true,type:r.is_default?'primary':'default',onClick:()=>setDefault(r)},{icon:()=>h(Star,{size:16,fill:r.is_default?'currentColor':'none'}),default:()=>r.is_default?'默认':'设为默认'}),h(NButton,{text:true,onClick:()=>router.push({path:'/dns',query:{zone:r.id}})},{icon:()=>h(ArrowRight,{size:16}),default:()=> '管理'})])}]
onMounted(async()=>{loading.value=true;try{await app.loadZones()}catch(e:any){message.error(e.message)}finally{loading.value=false}})
</script>
<template><div><div class="mb-5"><h1 class="page-title">域名</h1><p class="page-subtitle">来自所有 Cloudflare API Token 的可访问 Zone</p></div><div class="surface overflow-hidden"><n-data-table v-if="loading||app.zones.length" :columns="columns" :data="app.zones" :loading="loading" :row-key="(r:Zone)=>r.id" :bordered="false"/><n-empty v-else class="py-16" description="尚未同步域名"><template #extra><n-button type="primary" @click="router.push('/tokens')">添加 API Token</n-button></template></n-empty></div></div></template>
