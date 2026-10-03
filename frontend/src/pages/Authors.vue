<template>
  <div>
    <div class="panel" v-if="isEditor">
      <div class="panel-h">新建作者（可同时开通作者登录账号）</div>
      <div class="panel-b">
        <div class="row">
          <div><label>姓名 *</label><input v-model="form.name" /></div>
          <div><label>邮箱 *</label><input v-model="form.email" /></div>
        </div>
        <div class="row">
          <div><label>收款账户（模拟）</label><input v-model="form.bank_info" /></div>
          <div><label>登录密码（留空则不开通账号）</label><input v-model="form.password" /></div>
        </div>
        <div style="margin-top:12px"><button class="btn" @click="create">创建</button></div>
        <div v-if="err" class="alert err">{{ err }}</div>
      </div>
    </div>
    <div class="panel">
      <div class="panel-h">作者档案</div>
      <div class="panel-b">
        <table>
          <thead><tr><th>ID</th><th>姓名</th><th>邮箱</th><th>收款账户</th></tr></thead>
          <tbody>
            <tr v-for="a in authors" :key="a.id">
              <td>{{ a.id }}</td><td><b>{{ a.name }}</b></td><td>{{ a.email }}</td>
              <td class="small muted">{{ a.bank_info || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { api, useAuth } from '../auth.js'
const auth = useAuth()
const isEditor = computed(() => auth.user.value.role === 'editor')
const authors = ref([])
const form = ref({ name: '', email: '', bank_info: '', password: '' })
const err = ref('')
async function load() { authors.value = (await api('/authors')).items }
async function create() {
  err.value = ''
  try {
    await api('/authors', { method: 'POST', body: JSON.stringify(form.value) })
    form.value = { name: '', email: '', bank_info: '', password: '' }
    await load()
  } catch (e) { err.value = e.message }
}
onMounted(load)
</script>
