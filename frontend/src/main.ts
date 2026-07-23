import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { create, NAlert, NBadge, NButton, NCard, NCheckbox, NConfigProvider, NDataTable, NDialogProvider, NDivider, NDrawer, NDrawerContent, NDropdown, NEmpty, NForm, NFormItem, NGlobalStyle, NIcon, NInput, NInputNumber, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NMenu, NMessageProvider, NModal, NPagination, NPopconfirm, NRadioButton, NRadioGroup, NSelect, NSpace, NSpin, NSwitch, NTag, NThing, NTooltip } from 'naive-ui'
import App from './App.vue'
import { router } from './router'
import { useAppStore } from './stores/app'
import './style.css'

const pinia = createPinia()
const app = createApp(App)
app.use(pinia)
app.use(create({ components: [NAlert,NBadge,NButton,NCard,NCheckbox,NConfigProvider,NDataTable,NDialogProvider,NDivider,NDrawer,NDrawerContent,NDropdown,NEmpty,NForm,NFormItem,NGlobalStyle,NIcon,NInput,NInputNumber,NLayout,NLayoutContent,NLayoutHeader,NLayoutSider,NMenu,NMessageProvider,NModal,NPagination,NPopconfirm,NRadioButton,NRadioGroup,NSelect,NSpace,NSpin,NSwitch,NTag,NThing,NTooltip] }))
const store = useAppStore()
store.applyTheme()
await store.loadSession()
app.use(router)
window.addEventListener('flaredns:auth-expired', () => { store.clear(); if (router.currentRoute.value.name !== 'login') router.push({ name: 'login' }) })
app.mount('#app')
