<template>
  <div v-if="summary">
    <div class="grid3" style="margin-bottom:18px">
      <div class="stat"><div class="k">正常稿酬累计</div><div class="v">{{ yuan(summary.normal_total) }}</div></div>
      <div class="stat"><div class="k">补差累计（含退货负向）</div><div class="v" :class="summary.adjustment_total < 0 ? 't-neg' : ''">{{ yuan(summary.adjustment_total) }}</div></div>
      <div class="stat"><div class="k">已实际到账</div><div class="v" style="color:var(--ok)">{{ yuan(summary.all_time_payable) }}</div></div>
    </div>

    <div class="panel">
      <div class="panel-h">我的结算单</div>
      <div class="panel-b">
        <table>
          <thead><tr><th>#</th><th>结算期</th><th class="num">正常稿酬</th><th class="num">补差合计</th><th class="num">最终应付</th><th>状态</th><th></th></tr></thead>
          <tbody>
            <tr v-for="s in settlements" :key="s.id">
              <td>{{ s.id }}</td>
              <td class="mono">{{ s.period }}</td>
              <td class="num">{{ yuan(s.normal_amount) }}</td>
              <td class="num" :class="{ 't-neg': s.adjustment_total < 0 }">{{ s.adjustment_total ? yuan(s.adjustment_total) : '—' }}</td>
              <td class="num"><b>{{ yuan(s.payable_total) }}</b></td>
              <td><span class="badge" :class="STATUS_LABELS[s.status].cls">{{ STATUS_LABELS[s.status].label }}</span></td>
              <td><router-link class="btn ghost sm" :to="`/my/bills/${s.id}`">计算明细</router-link></td>
            </tr>
            <tr v-if="!settlements.length"><td colspan="7" class="muted">暂无结算单</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <div class="panel">
      <div class="panel-h">我的补差单（异常特批 / 差错更正 / 跨期退货）</div>
      <div class="panel-b">
        <table>
          <thead><tr><th>单号</th><th>期</th><th>原因</th><th>说明</th><th class="num">金额</th><th>状态</th><th>回执</th></tr></thead>
          <tbody>
            <tr v-for="a in adjustments" :key="a.id">
              <td class="mono small">{{ a.code }}</td>
              <td>{{ a.period }}</td>
              <td class="small">{{ reasonText(a.reason) }}</td>
              <td class="small muted">{{ a.memo }}</td>
              <td class="num" :class="{ 't-neg': a.amount < 0 }"><b>{{ yuan(a.amount) }}</b></td>
              <td><span class="badge" :class="STATUS_LABELS[a.status]?.cls">{{ STATUS_LABELS[a.status]?.label || a.status }}</span></td>
              <td class="small mono">{{ a.receipt?.receipt_no || '—' }}</td>
            </tr>
            <tr v-if="!adjustments.length"><td colspan="7" class="muted">暂无补差单</td></tr>
          </tbody>
        </table>
        <div class="alert info small" style="margin-top:10px">
          跨区销售、跨期退货、合同到期后补传的销量不会计入正常稿酬；经出版社核实特批后，会以独立补差单结算，此处均可查看依据。
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../auth.js'
import { yuan, STATUS_LABELS } from '../format.js'
const summary = ref(null)
const settlements = ref([])
const adjustments = ref([])
const REASONS = {
  error_correction: '差错更正', exception_allowed: '异常特批',
  late_sales: '到期/补传', cross_period_return: '跨期退货',
}
function reasonText(r) { return REASONS[r] || r }
onMounted(async () => {
  summary.value = await api('/my/summary')
  settlements.value = (await api('/my/settlements')).items
  adjustments.value = (await api('/my/adjustments')).items
})
</script>
