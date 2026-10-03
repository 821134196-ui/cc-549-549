<template>
  <div>
    <div class="panel">
      <div class="panel-h">按期导入销量（CSV 粘贴或手工录入）</div>
      <div class="panel-b">
        <div class="row">
          <div><label>批次号（唯一，重复导入自动幂等拒绝）*</label><input v-model="batchRef" placeholder="CH2026-10" /></div>
          <div><label>归属结算期 *</label><input v-model="period" placeholder="2026-10" /></div>
        </div>
        <label>备注 / 数据来源</label><input v-model="note" placeholder="10月渠道对账单" />
        <label>销量行（CSV：work_id,format,language,region,sale_date,doc_ref,return_ref,qty,unit_price(元)）</label>
        <textarea v-model="csv" rows="9" :placeholder="ph"></textarea>
        <div class="flex spread" style="margin-top:10px">
          <button class="btn ghost sm" @click="loadDemo">填入示例</button>
          <button class="btn" @click="submit" :disabled="loading">{{ loading ? '导入中…' : '导入并自动判定授权/异常' }}</button>
        </div>
        <div v-if="err" class="alert err">{{ err }}</div>
        <div v-if="result" class="alert ok">
          <template v-if="result.duplicate">
            ⚠ 批次 <b>{{ result.batch.batch_ref }}</b> 此前已导入（{{ result.batch.row_count }} 行），本次为<b>重复导入，已全部跳过，不重复计酬</b>。
          </template>
          <template v-else>
            ✔ 导入完成：正常 {{ result.normal }} 行、异常 {{ result.exceptions }} 行；正常行版税合计 <b>{{ yuan(result.royalty) }}</b>
          </template>
        </div>
        <div v-if="result && result.reasons?.length" style="margin-top:8px">
          <div class="small muted" style="margin-bottom:4px">判定明细：</div>
          <div v-for="(r, i) in result.reasons" :key="i" class="alert warn small" style="margin:4px 0">{{ r }}</div>
        </div>
        <div class="alert info small" style="margin-top:12px">
          退货行 qty 填负数；若退货发生月与归属期不同，系统自动列为「跨期退货」。销售区域/语种不在销售时合同版本内 → 「越区/越语种」。
          销售日晚于合同到期日 → 「到期后补传」。异常行一律隔离，不进入正常收入。
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '../auth.js'
import { yuan } from '../format.js'

const batchRef = ref('CH2026-10')
const period = ref('2026-10')
const note = ref('')
const csv = ref('')
const err = ref('')
const result = ref(null)
const loading = ref(false)
const ph = `1,print,zh,CN,2026-10-05,D-CN-1001,,20,50.00
1,ebook,en,JP,2026-10-06,D-JP-1002,,10,30.00
1,print,zh,CN,2026-08-01,RT-0801,D-CN-9000,-3,50.00`

function loadDemo() { csv.value = ph }

function parseCSV() {
  const rows = []
  for (const raw of csv.value.split(/\r?\n/)) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) continue
    const p = line.split(',').map(s => s.trim())
    if (p.length < 8) throw new Error(`行字段不足 8 个：${line}`)
    rows.push({
      work_id: Number(p[0]),
      format: p[1],
      language: p[2],
      region: p[3],
      sale_date: p[4],
      doc_ref: p[5] || '',
      return_ref: p[6] || '',
      qty: Number(p[7]),
      unit_price: Math.round(Number(p[8] || 0) * 100),
    })
  }
  if (!rows.length) throw new Error('没有可导入的行')
  return rows
}

async function submit() {
  err.value = ''; result.value = null
  try {
    const rows = parseCSV()
    loading.value = true
    result.value = await api('/sales/import', {
      method: 'POST',
      body: JSON.stringify({ batch_ref: batchRef.value, period: period.value, note: note.value, rows }),
    })
  } catch (e) { err.value = e.message } finally { loading.value = false }
}
</script>
