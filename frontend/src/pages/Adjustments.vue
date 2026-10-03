<template>
  <div>
    <div class="panel">
      <div class="panel-h">
        补差单（差错更正 / 异常特批 / 跨期退货 / 到期补传）
        <input v-model="period" placeholder="按期筛选" style="width:160px" @change="load" />
      </div>
      <div class="panel-b">
        <div class="alert info small">
          补差单独立于正常结算金额，可正可负（退货为负）；由异常销量特批生成的补差单可逐行追溯到原始销量。
          补差单同样需要「确认 → 付款」两步，付款后产生独立回执。
        </div>
        <table>
          <thead><tr>
            <th>单号</th><th>作者</th><th>期</th><th>原因</th><th>关联原单</th>
            <th class="num">金额</th><th>状态</th><th>回执</th><th></th>
          </tr></thead>
          <tbody>
            <tr v-for="a in list" :key="a.id">
              <td class="mono small">{{ a.code }}</td>
              <td>{{ a.author?.name }}</td>
              <td>{{ a.period }}</td>
              <td class="small">{{ reasonText(a.reason) }}</td>
              <td class="small muted">{{ a.original_settlement_id ? '结算单 #' + a.original_settlement_id : '无（独立）' }}</td>
              <td class="num" :class="{ 't-neg': a.amount < 0 }"><b>{{ yuan(a.amount) }}</b></td>
              <td><span class="badge" :class="STATUS_LABELS[a.status]?.cls">{{ STATUS_LABELS[a.status]?.label || a.status }}</span></td>
              <td class="small mono">{{ a.receipt?.receipt_no || '—' }}</td>
              <td class="small">
                <button v-if="a.status==='draft'" class="btn ghost sm" @click="confirm(a.id)">确认</button>
                <button v-if="a.status==='confirmed'" class="btn sm" @click="pay(a.id)">付款</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../auth.js'
import { yuan, STATUS_LABELS } from '../format.js'
const list = ref([])
const period = ref('')
const REASONS = {
  error_correction: '差错更正', exception_allowed: '异常特批',
  late_sales: '到期/补传', cross_period_return: '跨期退货',
}
function reasonText(r) { return REASONS[r] || r }
async function load() {
  list.value = (await api('/adjustments' + (period.value ? '?period=' + period.value : ''))).items
}
async function confirm(id) { await api(`/adjustments/${id}/confirm`, { method: 'POST' }); await load() }
async function pay(id) {
  try { await api('/payments/pay', { method: 'POST', body: JSON.stringify({ type: 'adjustment', id }) }); await load() }
  catch (e) { alert(e.message) }
}
onMounted(load)
</script>
