import { reactive, computed } from 'vue'

const STORE_KEY = 'royalty_auth'

function load() {
  try {
    return JSON.parse(localStorage.getItem(STORE_KEY) || 'null')
  } catch {
    return null
  }
}

const state = reactive({ session: load() })

export function useAuth() {
  const user = computed(() => state.session?.user || null)
  const token = computed(() => state.session?.token || '')

  function setSession(s) {
    state.session = s
    localStorage.setItem(STORE_KEY, JSON.stringify(s))
  }
  function logout() {
    state.session = null
    localStorage.removeItem(STORE_KEY)
  }
  function homePath() {
    const role = user.value?.role
    if (role === 'finance') return '/dashboard'
    if (role === 'editor') return '/works'
    if (role === 'author') return '/my/bills'
    return '/login'
  }
  return { user, token, setSession, logout, homePath }
}

export async function api(path, options = {}) {
  const { token } = useAuth()
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
  if (token.value) headers.Authorization = `Bearer ${token.value}`
  const res = await fetch('/api' + path, { ...options, headers })
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { raw: text }
  }
  if (!res.ok) {
    const err = new Error(data?.error || `请求失败 (${res.status})`)
    err.status = res.status
    err.data = data
    throw err
  }
  return data
}
