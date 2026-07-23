import { api } from './api'

const decode = (value: string) => {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - value.length % 4) % 4)
  return Uint8Array.from(atob(base64), c => c.charCodeAt(0))
}
const encode = (value: ArrayBuffer | null) => {
  if (!value) return null
  const bytes = new Uint8Array(value)
  let binary = ''
  bytes.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}
const prepareCreation = (raw: any): PublicKeyCredentialCreationOptions => {
  const options = raw.publicKey || raw
  return { ...options, challenge: decode(options.challenge), user: { ...options.user, id: decode(options.user.id) }, excludeCredentials: options.excludeCredentials?.map((item: any) => ({ ...item, id: decode(item.id) })) }
}
const prepareRequest = (raw: any): PublicKeyCredentialRequestOptions => {
  const options = raw.publicKey || raw
  return { ...options, challenge: decode(options.challenge), allowCredentials: options.allowCredentials?.map((item: any) => ({ ...item, id: decode(item.id) })) }
}
const assertionJSON = (credential: PublicKeyCredential) => {
  const response = credential.response as AuthenticatorAssertionResponse
  return { id: credential.id, rawId: encode(credential.rawId), type: credential.type, response: { authenticatorData: encode(response.authenticatorData), clientDataJSON: encode(response.clientDataJSON), signature: encode(response.signature), userHandle: encode(response.userHandle) }, clientExtensionResults: credential.getClientExtensionResults(), authenticatorAttachment: credential.authenticatorAttachment }
}
const creationJSON = (credential: PublicKeyCredential) => {
  const response = credential.response as AuthenticatorAttestationResponse
  return { id: credential.id, rawId: encode(credential.rawId), type: credential.type, response: { attestationObject: encode(response.attestationObject), clientDataJSON: encode(response.clientDataJSON), transports: response.getTransports?.() || [] }, clientExtensionResults: credential.getClientExtensionResults(), authenticatorAttachment: credential.authenticatorAttachment }
}
export async function loginWithPasskey(reauth = false) {
  if (!window.PublicKeyCredential || !window.isSecureContext) throw new Error('当前地址不支持 Passkey，请使用 HTTPS 或 localhost')
  const begin = await api.request<any>('/auth/passkey/login/options', { method: 'POST', body: '{}' })
  const credential = await navigator.credentials.get({ publicKey: prepareRequest(begin.options) }) as PublicKeyCredential | null
  if (!credential) throw new Error('未选择 Passkey')
  return api.request<any>('/auth/passkey/login/finish', { method: 'POST', headers: { 'X-WebAuthn-Challenge-ID': begin.challenge_id, ...(reauth ? { 'X-Reauth': '1' } : {}) }, body: JSON.stringify(assertionJSON(credential)) })
}
export async function registerPasskey(name: string) {
  if (!window.PublicKeyCredential || !window.isSecureContext) throw new Error('当前地址不支持 Passkey，请使用 HTTPS 或 localhost')
  const begin = await api.request<any>('/passkeys/register/options', { method: 'POST', body: '{}' })
  const credential = await navigator.credentials.create({ publicKey: prepareCreation(begin.options) }) as PublicKeyCredential | null
  if (!credential) throw new Error('Passkey 创建已取消')
  return api.request<any>('/passkeys/register/finish', { method: 'POST', headers: { 'X-WebAuthn-Challenge-ID': begin.challenge_id, 'X-Passkey-Name': name }, body: JSON.stringify(creationJSON(credential)) })
}
