<template>
  <div v-if="s">
    <div class="flex" style="margin-bottom:14px">
      <router-link to="/settlements" class="muted small">← 返回结算单列表</router-link>
    </div>

    <div class="panel">
      <div class="panel-h">
        <div>结算单 #{{ s.id }} · <b>{{ s.author.name }}</b> · <span class="mono">{{ s.period }}</span></div>
        <span class="badge" :class="STATUS_LABELS[s.status].cls">{{ STATUS_LABELS[s.status].label }}</span>
      </div>
      <div class="panel-b">
        <div class="grid3">
          <div class="stat"><div class="k">正常稿酬（已冻结值）</div><div class="v">{{ yuan(s.normal_amount) }}</div></div>
          <div class="stat"><div class="k">补差合计（关联本单）</div><div class="v" :class="s.adjustment_total < 0 ? 't-neg' : ''">{{ yuan(s.adjustment_total) }}</div></div>
          <div class="stat"><div class="k">最终应付</div><div class="v">{{ yuan(s.payable_total) }}</div></div>
        </div>
        <div class="flex" style="margin-top:14px">
          <button v-if="s.status==='draft'" class="btn gold" @click="confirmSt">确认结算单（金额冻结，之后不可改）</button>
          <button v-if="s.status==='confirmed'" class="btn" @click="paySt">模拟付款</button>
          <span v-if="s.status==='paid'" class="alert ok mb0" style="margin:0">已于 {{ s.paid_at?.replace('T',' ').slice(0,19) }} 付款</span>
        </div>
        <div v-if="s.receipt" class="alert ok small" style="margin-top:12px">
          付款回执：<b class="mono">{{ s.receipt.receipt_no }}</b>｜{{ s.receipt.channel }}｜金额 {{ yuan(s.receipt.amount) }}｜{{ s.receipt.memo }}
        </div>
        <div v-if="msg" class="alert ok small" style="margin-top:10px">{{ msg }}</div>
        <div v-if="err" class="alert err small" style="margin-top:10px">{{ err }}</div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-h">计酬明细（{{ s.entries.length }} 条）— 每条可追溯原始销量与抵扣依据</div>
      <div class="panel-b">
        <table>
          <thead><tr>
            <th>作品</th><th>销售日</th><th>区域</th><th>格式</th><th>类型</th>
            <th class="num">数量</th><th class="num">原始销量金额</th><th>合同版本</th>
            <th class="num">版税总额</th><th class="num">我的分成</th><th class="num">应付稿酬</th><th>原始销量行</th>
          </tr></thead>
          <tbody>
            <tr v-for="e in s.entries" :key="e.id" :class="{ 't-neg': e.royalty < 0 }">
              <td>#{{ e.work_id }}</td>
              <td class="small">{{ e.sale_date }}</td>
              <td>{{ e.region }}</td>
              <td>{{ e.format === 'print' ? '纸书' : '电子书' }}</td>
              <td>
                <span v-if="e.kind==='return'" class="badge ex">退货抵扣</span>
                <span v-else class="badge ok">销售</span>
              </td>
              <td class="num">{{ e.qty }}</td>
              <td class="num">{{ yuan(e.amount) }}</td>
              <td class="small">合同 #{{ e.contract_id }}<br /><span class="muted">{{ (e.share_bps/100).toFixed(2).replace(/\.?0+$/,'') }}%</span></td>
              <td class="num">{{ yuan(e.royalty_gross) }}</td>
              <td class="num">{{ (e.share_bps/100).toFixed(2).replace(/\.?0+$/,'') }}%</td>
              <td class="num"><b>{{ yuan(e.royalty) }}</b></td>
              <td class="small mono">line #{{ e.sales_line_id }}</td>
            </tr>
            <tr class="total-row">
              <td colspan="10">合计（正常收入）</td>
              <td class="num">{{ yuan(s.normal_amount) }}</td>
              <td></td>
            </tr>
          </tbody>
        </table>
        <div class="alert info small" style="margin-top:10px">
          抵扣依据：每行记录了销售发生当日有效的合同版本（contract_id）与该作者的分成比例。退货行以负数抵扣，跨期退货不在此表，而在「补差单」中单独处理。
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-h">关联补差单（{{ s.related_adjustments.length }}）</div>
      <div class="panel-b">
        <table v-if="s.related_adjustments.length">
          <thead><tr><th>单号</th><th>期</th><th>原因</th><th>说明</th><th class="num">金额</th><th>状态</th><th></th></tr></thead>
          <tbody>
            <tr v-for="a in s.related_adjustments" :key="a.id">
              <td class="mono small">{{ a.code }}</td>
              <td>{{ a.period }}</td>
              <td class="small">{{ reasonText(a.reason) }}</td>
              <td class="small muted">{{ a.memo }}</td>
              <td class="num" :class="{ 't-neg': a.amount < 0 }"><b>{{ yuan(a.amount) }}</b></td>
              <td><span class="badge" :class="STATUS_LABELS[a.status]?.cls">{{ STATUS_LABELS[a.status]?.label || a.status }}</span></td>
              <td class="small">
                <button v-if="a.status==='draft'" class="btn ghost sm" @click="confirmAdj(a.id)">确认</button>
                <button v-if="a.status==='confirmed'" class="btn sm" @click="payAdj(a.id)">付款</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-else class="muted small">暂无补差单。已确认结算单如发现差错或处理异常销量，请在下方开立补差单——原单金额保持冻结不变。</div>
      </div>
    </div>

    <div class="panel" v-if="s.status !== 'draft'">
      <div class="panel-h">差错更正 / 异常补付（生成关联本单的补差单）</div>
      <div class="panel-b">
        <div class="row">
          <div>
            <label>原因类型</label>
            <select v-model="adjForm.reason">
              <option value="error_correction">差错更正</option>
              <option value="exception_allowed">异常特批</option>
              <option value="late_sales">到期/补传销量</option>
              <option value="cross_period_return">跨期退货</option>
            </select>
          </div>
          <div><label>金额（元，可为负数）</label><input type="number" step="0.01" v-model="adjAmountYuan" /></div>
        </div>
        <label>说明（作者可见，写明差错原因/原始单据）</label>
        <input v-model="adjForm.memo" placeholder="如：8月对账单单价录入有误，应补 …" />
        <div style="margin-top:12px"><button class="btn gold" @click="createAdj">开立补差单</button></div>
        <div v-if="adjErr" class="alert err small">{{ adjErr }}</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed, reactive } from 'vue'
import { api } from '../auth.js'
import { yuan, STATUS_LABELS } from '../format.js'

const props = defineProps({ id: String })
const s = ref(null)
const msg = ref('')
const err = ref('')
const adjErr = ref('')
const adjAmountYuan = ref(0)
const adjForm = reactive({ reason: 'error_correction', memo: '' })

const REASONS = {
  error_correction: '差错更正', exception_allowed: '异常特批',
  late_sales: '到期/补传销量', cross_period_return: '跨期退货',
}
function reasonText(r) { return REASONS[r] || r }

async function load() { s.value = await api(`/settlements/${props.id}`) }
async function confirmSt() {
  err.value = ''
  try { await api(`/settlements/${props.id}/confirm`, { method: 'POST' }); await load(); msg.value = '已确认，金额冻结。' }
  catch (e) { err.value = e.message }
}
async function paySt() {
  err.value = ''
  try { await api('/payments/pay', { method: 'POST', body: JSON.stringify({ type: 'settlement', id: Number(props.id) }) }); await load() }
  catch (e) { err.value = e.message }
}
async function confirmAdj(id) {
  await api(`/adjustments/${id}/confirm`, { method: 'POST' }); await load()
}
async function payAdj(id) {
  err.value = ''
  try { await api('/payments/pay', { method: 'POST', body: JSON.stringify({ type: 'adjustment', id }) }); await load() }
  catch (e) { err.value = e.message }
}
async function createAdj() {
  adjErr.value = ''
  const amount = Math.round(Number(adjAmountYuan.value || 0) * 100)
  if (!amount) { adjErr.value = '金额不能为 0'; return }
  try {
    await api('/adjustments', {
      method: 'POST',
      body: JSON.stringify({
        original_settlement_id: Number(props.id), author_id: s.value.author_id,
        period: s.value.period, reason: adjForm.reason, memo: adjForm.memo, amount,
      }),
    })
    adjForm.memo = ''; adjAmountYuan.value = 0
    await load()
  } catch (e) { adjErr.value = e.message }
}
onMounted(load)
</script>
