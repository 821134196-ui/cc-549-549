import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import './styles.css'

import Login from './pages/Login.vue'
import Works from './pages/Works.vue'
import WorkDetail from './pages/WorkDetail.vue'
import Authors from './pages/Authors.vue'
import ImportSales from './pages/ImportSales.vue'
import Batches from './pages/Batches.vue'
import BatchDetail from './pages/BatchDetail.vue'
import SalesLines from './pages/SalesLines.vue'
import Settlements from './pages/Settlements.vue'
import SettlementDetail from './pages/SettlementDetail.vue'
import Adjustments from './pages/Adjustments.vue'
import Payments from './pages/Payments.vue'
import Dashboard from './pages/Dashboard.vue'
import MyBills from './pages/MyBills.vue'
import MyBillDetail from './pages/MyBillDetail.vue'
import { useAuth } from './auth.js'

const routes = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', component: Dashboard, meta: { roles: ['finance'] } },
  { path: '/works', component: Works },
  { path: '/works/:id', component: WorkDetail, props: true },
  { path: '/authors', component: Authors, meta: { roles: ['editor'] } },
  { path: '/sales/import', component: ImportSales, meta: { roles: ['finance'] } },
  { path: '/batches', component: Batches, meta: { roles: ['finance'] } },
  { path: '/batches/:id', component: BatchDetail, props: true, meta: { roles: ['finance'] } },
  { path: '/sales/lines', component: SalesLines, meta: { roles: ['finance'] } },
  { path: '/settlements', component: Settlements, meta: { roles: ['finance'] } },
  { path: '/settlements/:id', component: SettlementDetail, props: true, meta: { roles: ['finance'] } },
  { path: '/adjustments', component: Adjustments, meta: { roles: ['finance'] } },
  { path: '/payments', component: Payments, meta: { roles: ['finance'] } },
  { path: '/my/bills', component: MyBills, meta: { roles: ['author'] } },
  { path: '/my/bills/:id', component: MyBillDetail, props: true, meta: { roles: ['author'] } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuth()
  if (to.meta.public) return true
  if (!auth.user.value) return '/login'
  if (to.meta.roles && !to.meta.roles.includes(auth.user.value.role)) {
    return auth.homePath()
  }
  return true
})

createApp(App).use(router).mount('#app')
