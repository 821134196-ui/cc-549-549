<template>
  <template v-if="isLogin"><router-view /></template>
  <div v-else class="layout">
    <aside class="sidebar">
      <div class="logo">墨衡<small>ROYALTY LEDGER</small></div>

      <template v-if="role === 'finance'">
        <nav class="nav-group">
          <h4>财务</h4>
          <router-link class="nav-item" :class="{active: $route.path==='/dashboard'}" to="/dashboard">结算总览</router-link>
          <router-link class="nav-item" :class="{active: $route.path.startsWith('/sales')}" to="/sales/import">导入销量</router-link>
          <router-link class="nav-item" :class="{active: $route.path.startsWith('/batches')}" to="/batches">导入批次</router-link>
          <router-link class="nav-item" :class="{active: $route.path==='/sales/lines'}" to="/sales/lines">销量与异常</router-link>
          <router-link class="nav-item" :class="{active: $route.path.startsWith('/settlements')}" to="/settlements">结算单</router-link>
          <router-link class="nav-item" :class="{active: $route.path.startsWith('/adjustments')}" to="/adjustments">补差单</router-link>
          <router-link class="nav-item" :class="{active: $route.path==='/payments'}" to="/payments">付款回执</router-link>
        </nav>
      </template>

      <template v-if="role === 'editor'">
        <nav class="nav-group">
          <h4>编辑</h4>
          <router-link class="nav-item" :class="{active: $route.path.startsWith('/works')}" to="/works">作品与合同</router-link>
          <router-link class="nav-item" :class="{active: $route.path==='/authors'}" to="/authors">作者档案</router-link>
        </nav>
      </template>

      <template v-if="role === 'author'">
        <nav class="nav-group">
          <h4>我的稿酬</h4>
          <router-link class="nav-item" :class="{active: $route.path.startsWith('/my')}" to="/my/bills">我的账单</router-link>
        </nav>
      </template>

      <nav class="nav-group">
        <h4>公共</h4>
        <router-link class="nav-item" :class="{active: $route.path.startsWith('/works') && role!=='editor'}" to="/works">作品授权查询</router-link>
      </nav>

      <div class="spacer"></div>
      <div class="userbox">
        <b>{{ auth.user.value?.name }}</b><br />
        <span class="role">{{ roleLabel }}</span><br />
        <span class="small" style="color:#a99a80">{{ auth.user.value?.email }}</span><br />
        <button @click="logout">退出登录</button>
      </div>
    </aside>
    <main class="main">
      <div class="topbar"><h1>{{ title }}</h1><span class="muted small">金额单位：人民币元（后端按整数分记账）</span></div>
      <div class="content"><router-view /></div>
    </main>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuth } from './auth.js'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const isLogin = computed(() => route.path === '/login')
const role = computed(() => auth.user.value?.role)
const roleLabels = { editor: '编辑', finance: '财务', author: '作者' }
const roleLabel = computed(() => roleLabels[role.value] || '')
const titles = {
  '/dashboard': '结算总览', '/sales/import': '按期导入销量', '/batches': '导入批次',
  '/sales/lines': '销量与异常', '/settlements': '结算单', '/adjustments': '补差单',
  '/payments': '付款回执（本地模拟）', '/works': '作品与合同', '/authors': '作者档案',
  '/my/bills': '我的账单',
}
const title = computed(() => {
  if (route.path.startsWith('/works')) return '作品与授权合同'
  if (route.path.startsWith('/settlements')) return '结算单'
  if (route.path.startsWith('/batches')) return '销量导入批次'
  if (route.path.startsWith('/adjustments')) return '补差单'
  if (route.path.startsWith('/my')) return '我的稿酬账单'
  if (route.path.startsWith('/sales')) return '销量'
  return titles[route.path] || '墨衡稿酬系统'
})
function logout() {
  auth.logout()
  router.push('/login')
}
</script>
