<script setup lang="ts">
import { computed } from 'vue'
import { darkTheme, zhCN, dateZhCN, type GlobalThemeOverrides } from 'naive-ui'
import { useAppStore } from '@/stores/app'
import AppShell from '@/components/AppShell.vue'
const app = useAppStore()
const overrides: GlobalThemeOverrides = {
  common: { primaryColor: '#f6821f', primaryColorHover: '#fb923c', primaryColorPressed: '#d8670f', primaryColorSuppl: '#f6821f', borderRadius: '6px', borderRadiusSmall: '4px' },
  Menu: {
    itemColorActiveInverted: '#f6821f',
    itemColorActiveHoverInverted: '#fb923c',
    itemColorActiveCollapsedInverted: '#f6821f',
    itemTextColorActiveInverted: '#ffffff',
    itemTextColorActiveHoverInverted: '#ffffff',
    itemIconColorActiveInverted: '#ffffff',
    itemIconColorActiveHoverInverted: '#ffffff',
  },
}
const theme = computed(() => app.dark ? darkTheme : null)
</script>
<template><n-config-provider :theme="theme" :theme-overrides="overrides" :locale="zhCN" :date-locale="dateZhCN"><n-global-style/><n-dialog-provider><n-message-provider><AppShell v-if="app.authenticated"/><router-view v-else/></n-message-provider></n-dialog-provider></n-config-provider></template>
