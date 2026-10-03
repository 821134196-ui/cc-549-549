<template>
  <div>
    <div class="panel">
      <div class="panel-h">
        结算单（一位作者 × 一个结算期）
        <div class="flex">
          <input v-model="period" placeholder="生成/筛选期 2026-08" style="width:170px" @change="load" />
          <button class="btn gold sm" @click="generate" :disabled="busy">{{ busy ? '计酬中…' : '按所选期计酬' }}</button>
          <select v-model="statusFilter" @change="load" style="width:130px">
            <option value="">全部状态</option>
            <option value="draft">草稿</option>
            <option value="confirmed">已确认</option>
            <option value="paid">已付款</option>
          </select>
        </div>
      </div>
      <div class="panel-b">
        <div v-if="msg" class="alert ok small">{{ msg }}</div>
        <div class="alert info small">
          计酬时按<b>每笔销售发生当日有效</b>的合同版本匹配费率与作者分成；一条销量对一位作者只计酬一次（重复导入/重复计酬均有数据库唯一约束兜底）。
        </div>
        <table>
          <thead><tr>
            <th>ID</th><th>结算期</th><th>作者</th><th class="num">正常稿酬</th>
            <th class="num">补差合计</th><th class="num">最终应付</th><th>状态</th><th>关联补差</th><th></th>
          </tr></thead>
          <tbody>
            <tr v-for="s in list" :key="s.id">
              <td>#{{ s.id }}</td>
              <td class="mono">{{ s.period }}</td>
              <td><b>{{ s.author.name }}</b></td>
              <td class="num">{{ yuan(s.normal_amount) }}</td>
              <td class="num" :class="{ 't-neg': s.adjustment_total < 0 }">{{ s.adjustment_total ? yuan(s.adjustment_total) : '—' }}</td>
              <td class="num"><b>{{ yuan(s.payable_total) }}</b></td>
              <td><span class="badge" :class="STATUS_LABELS[s.status].cls">{{ STATUS_LABELS[s.status].label }}</span></td>
              <td class="small">{{ s.related_adjustments.length || '—' }}</td>
              <td><router-link class="btn ghost sm" :to="`/settlements/${s.id}`">详情</router-link></td>
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
const period = ref('2026-08')
const statusFilter = ref('')
const msg = ref('')
const busy = ref(false)
async function load() {
  const p = new URLSearchParams()
  if (period.value) p.set('period', period.value)
  if (statusFilter.value) p.set('status', statusFilter.value)
  list.value = (await api('/settlements?' + p)).items
}
async function generate() {
  busy.value = true; msg.value = ''
  try {
    const r = await api('/settlements/generate', { method: 'POST', body: JSON.stringify({ period: period.value }) })
    const n = Object.keys(r.affected_authors || {}).length
    msg.value = `计酬完成：涉及 ${n} 位作者；本期异常 ${r.exception_lines} 行已隔离（请到“销量与异常”处理）。`
    await load()
  } catch (e) { msg.value = '失败：' + e.message } finally { busy.value = false }
}
onMounted(load)
</script>
