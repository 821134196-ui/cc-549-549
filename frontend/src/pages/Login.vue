<template>
  <div class="login-wrap">
    <div class="login-card">
      <h1>墨衡</h1>
      <div class="sub">出版社稿酬结算与授权区域系统</div>
      <div v-if="error" class="alert err">{{ error }}</div>
      <label>邮箱</label>
      <input v-model="email" placeholder="name@pub.com" @keyup.enter="login" />
      <label>密码</label>
      <input v-model="password" type="password" @keyup.enter="login" />
      <div style="margin-top:18px"><button class="btn" style="width:100%" :disabled="loading" @click="login">{{ loading ? '登录中…' : '登 录' }}</button></div>
      <div class="hint">
        示例账号（密码见下）：<br />
        编辑 <code>editor@pub.com / editor123</code><br />
        财务 <code>finance@pub.com / finance123</code><br />
        作者 <code>lin@author.com / author123</code>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, useAuth } from '../auth.js'

const email = ref('finance@pub.com')
const password = ref('finance123')
const error = ref('')
const loading = ref(false)
const auth = useAuth()
const router = useRouter()

async function login() {
  error.value = ''
  loading.value = true
  try {
    const s = await api('/auth/login', { method: 'POST', body: JSON.stringify({ email: email.value, password: password.value }) })
    auth.setSession(s)
    router.push(auth.homePath())
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>
