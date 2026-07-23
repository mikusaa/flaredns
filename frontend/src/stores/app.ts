import { defineStore } from 'pinia'
import { api, setCSRFToken } from '@/api'

export interface Session { username: string; csrf_token: string; expires_at: string; passkey_count: number; public_url: string; rp_id: string }
export interface Zone { id: number; api_token_id: number; token_name: string; cloudflare_id: string; name: string; status: string; record_count: number; is_default: boolean; last_synced_at?: string }

export const useAppStore = defineStore('app', {
  state: () => ({ session: null as Session | null, zones: [] as Zone[], zonesLoaded: false, dark: localStorage.getItem('flaredns-theme') === 'dark' }),
  getters: { authenticated: state => !!state.session, defaultZone: state => state.zones.find(zone => zone.is_default) || state.zones[0] },
  actions: {
    async loadSession() { try { this.session = await api.request<Session>('/auth/session'); setCSRFToken(this.session.csrf_token) } catch { this.session = null; setCSRFToken('') } },
    setSession(session: Session) { this.session = session; setCSRFToken(session.csrf_token) },
    async loadZones() { this.zones = await api.request<Zone[]>('/zones'); this.zonesLoaded = true },
    clear() { this.session = null; this.zones = []; this.zonesLoaded = false; setCSRFToken('') },
    toggleTheme() { this.dark = !this.dark; localStorage.setItem('flaredns-theme', this.dark ? 'dark' : 'light'); document.documentElement.classList.toggle('dark', this.dark) },
    applyTheme() { document.documentElement.classList.toggle('dark', this.dark) },
  },
})
