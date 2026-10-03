<template>
  <div>
    <div class="panel">
      <div class="panel-h">
        销量明细与异常隔离
        <div class="flex">
          <select v-model="status" @change="load" style="width:150px">
            <option value="">全部状态</option>
            <option value="exception">仅异常隔离</option>
            <option value="normal">正常待计酬</option>
            <option value="settled">已计酬</option>
          </select>
          <input v-model="period" placeholder="归属期 2026-08" style="width:160px" @change="load" />
        </div>
      </div>
      <div class="panel-b">
        <div class="alert warn small">
          以下异常销量<b>不会混入正常收入</b>：越区销售、未授权语种、跨期退货、合同到期后补传、结算单确认后补传。
          核实后可点「特批计酬」，系统会生成<b>关联原单的补差单</b>（退货为负向补差）。
        </div>
        <table>
          <thead><tr>
            <th>ID</th><th>作品</th><th>格式</th><th>区域/语种</th><th>销售日</th>
            <th class="num">数量</th><th class="num">金额</th><th class="num">版税估算</th>
            <th>判定 / 依据</th><th>状态</th><th></th>
          </tr></thead>
          <tbody>
            <tr v-for="l in lines" :key="l.id">
              <td>{{ l.id }}</td>
              <td>#{{ l.work_id }}</td>
              <td>{{ l.format === 'print' ? '纸书' : '电子书' }}</td>
              <td><b>{{ l.region }}</b> / {{ l.language }}</td>
              <td class="small">{{ l.sale_date }}</td>
              <td class="num" :class="{ 't-neg': l.qty < 0 }">{{ l.qty }}</td>
              <td class="num" :class="{ 't-neg': l.amount < 0 }">{{ yuan(l.amount) }}</td>
              <td class="num" :class="{ 't-neg': l.royalty_gross < 0 }">{{ yuan(l.royalty_gross) }}</td>
              <td style="max-width:300px">
                <span v-if="l.status==='exception'" class="badge ex">{{ EXCEPTION_LABELS[l.exception_type]?.label || l.exception_type }}</span>
                <span v-else class="badge ok">{{ LINE_STATUS[l.status]?.label }}</span>
                <div class="small muted">{{ l.exception_memo }}</div>
                <div class="small muted">合同版本：{{ l.contract_id ? '#' + l.contract_id : '无' }}　单据：<span class="mono">{{ l.doc_ref }}</span></div>
              </td>
              <td>
                <span v-if="l.resolved" class="badge ok">已处理</span>
                <span v-else-if="l.status==='exception'" class="badge st-draft">待处理</span>
              </td>
              <td>
                <button v-if="l.status==='exception' && !l.resolved" class="btn gold sm" @click="openAllow(l)">特批计酬</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="target" class="panel" style="border:2px solid var(--gold)">
      <div class="panel-h">异常特批：为销量 #{{ target.id }} 生成补差单（不进正常收入）</div>
      <div class="panel-b">
        <div class="alert warn small">{{ target.exception_memo }}</div>
        <div class="row">
          <div><label>补差归属期</label><input v-model="allow.period" /></div>
        </div>
        <label>特批说明（会写入补差单，作者可见）</label>
        <input v-model="allow.memo" placeholder="经渠道核实，同意按销售时合同版本计酬" />
        <div class="flex" style="margin-top:12px">
          <button class="btn" @click="submitAllow">按销售时合同版本分成，生成补差单</button>
          <button class="btn ghost" @click="target=null">取消</button>
        </div>
        <div v-if="allowMsg" class="alert ok">{{ allowMsg }}</div>
        <div v-if="allowErr" class="alert err">{{ allowErr }}</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../auth.js'
import { yuan, EXCEPTION_LABELS, LINE_STATUS } from '../format.js'

const lines = ref([])
const status = ref('exception')
const period = ref('')
const target = ref(null)
const allow = ref({ period: '', memo: '' })
const allowMsg = ref('')
const allowErr = ref('')

async function load() {
  const p = new URLSearchParams()
  if (status.value) p.set('status', status.value)
  if (period.value) p.set('period', period.value)
  lines.value = (await api('/sales/lines?' + p)).items
}
function openAllow(l) {
  target.value = l
  allow.value = { period: l.sale_date.slice(0, 7), memo: '' }
  allowMsg.value = ''; allowErr.value = ''
}
async function submitAllow() {
  allowErr.value = ''; allowMsg.value = ''
  try {
    const r = await api(`/exceptions/${target.value.id}/allow`, {
      method: 'POST', body: JSON.stringify(allow.value),
    })
    const n = r.adjustments?.length || 0
    allowMsg.value = `已生成 ${n} 张补差草稿单，版税总额 ${yuan(r.gross)}。请到「补差单」页面确认后付款。`
    await load()
  } catch (e) { allowErr.value = e.message }
}
onMounted(load)
</script>
