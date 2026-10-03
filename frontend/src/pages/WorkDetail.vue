<template>
  <div v-if="data">
    <div class="flex" style="margin-bottom:14px">
      <router-link to="/works" class="muted small">← 返回作品列表</router-link>
    </div>

    <div class="panel">
      <div class="panel-h">
        <div><b style="font-size:16px">{{ data.work.title }}</b> <span class="muted small mono">{{ data.work.isbn }}</span></div>
        <span class="muted small">{{ data.work.description }}</span>
      </div>
      <div class="panel-b">
        <table>
          <thead><tr><th>版本</th><th>有效期限（按销售发生日匹配）</th><th>授权语种</th><th>授权销售区域</th><th>纸书费率</th><th>电子书费率</th><th>作者分成</th><th>备注</th></tr></thead>
          <tbody>
            <tr v-for="v in data.versions" :key="v.id">
              <td><b>v{{ v.version_no }}</b></td>
              <td class="small">{{ v.effective_from }}<br />～ <span :class="v.effective_to > '2900-01-01' ? 't-muted' : ''">{{ v.effective_to > '2900-01-01' ? '长期有效' : v.effective_to }}</span></td>
              <td class="small">{{ v.languages }}</td>
              <td><b>{{ v.regions }}</b></td>
              <td class="small">{{ rateText(v.print_rate_type, v.print_rate) }}</td>
              <td class="small">{{ rateText(v.ebook_rate_type, v.ebook_rate) }}</td>
              <td class="small">
                <span v-for="s in v.shares" :key="s.id" class="mono">{{ s.author.name }} {{ (s.share_bps/100).toFixed(2).replace(/\.?0+$/,'') }}%　</span>
              </td>
              <td class="small muted">{{ v.note }}</td>
            </tr>
          </tbody>
        </table>
        <div class="alert info small" style="margin-top:12px">
          历史合同版本永久保留、不可修改。新建版本若生效日落在现行版本有效期内，系统会自动把旧版本截断到新生效日。
        </div>
      </div>
    </div>

    <div class="grid2">
      <div class="panel" v-if="isEditor">
        <div class="panel-h">新建合同版本</div>
        <div class="panel-b">
          <label>生效日 *（YYYY-MM-DD）</label>
          <input v-model="cf.effective_from" placeholder="2026-07-01" />
          <div class="row">
            <div><label>授权语种 *（逗号分隔）</label><input v-model="cf.languages" placeholder="zh,en" /></div>
            <div><label>授权销售区域 *</label><input v-model="cf.regions" placeholder="CN,US,GB" /></div>
          </div>
          <div class="row">
            <div>
              <label>纸书费率类型</label>
              <select v-model="cf.print_rate_type">
                <option value="percent">按销售额百分比（bps，6%=600）</option>
                <option value="per_copy">按册（分/册，2元=200）</option>
              </select>
            </div>
            <div><label>纸书费率值</label><input type="number" v-model.number="cf.print_rate" /></div>
          </div>
          <div class="row">
            <div>
              <label>电子书费率类型</label>
              <select v-model="cf.ebook_rate_type">
                <option value="percent">按销售额百分比（bps）</option>
                <option value="per_copy">按册（分/册）</option>
              </select>
            </div>
            <div><label>电子书费率值</label><input type="number" v-model.number="cf.ebook_rate" /></div>
          </div>

          <label>作者分成（合计必须 100% = 10000 bps）</label>
          <div v-for="(s, i) in cf.shares" :key="i" class="flex" style="margin-bottom:6px">
            <select v-model.number="s.author_id" style="flex:2">
              <option :value="0" disabled>选择作者</option>
              <option v-for="a in authors" :value="a.id" :key="a.id">{{ a.name }}（{{ a.email }}）</option>
            </select>
            <input type="number" v-model.number="s.share_bps" placeholder="bps，如 5000" style="flex:1" />
            <button class="btn danger sm" @click="cf.shares.splice(i,1)">删</button>
          </div>
          <div class="flex spread" style="margin-top:6px">
            <button class="btn ghost sm" @click="cf.shares.push({author_id:0, share_bps:0})">+ 增加作者</button>
            <span class="small" :style="{color: shareSum===10000 ? 'var(--ok)' : 'var(--ex)'}">合计 {{ (shareSum/100).toFixed(2) }}%</span>
          </div>

          <label>备注</label>
          <input v-model="cf.note" />
          <div style="margin-top:12px"><button class="btn" @click="submit">保存新版本</button></div>
          <div v-if="err" class="alert err">{{ err }}</div>
          <div v-if="okmsg" class="alert ok">{{ okmsg }}</div>
        </div>
      </div>

      <div class="panel">
        <div class="panel-h">授权范围直接查询</div>
        <div class="panel-b">
          <div class="row">
            <div><label>查询日期</label><input type="date" v-model="q.date" /></div>
            <div><label>销售区域</label><input v-model="q.region" placeholder="JP" /></div>
            <div><label>语种</label><input v-model="q.lang" placeholder="en" /></div>
          </div>
          <div style="margin-top:12px"><button class="btn gold" @click="queryLicense">查询该笔销售是否在授权内</button></div>
          <div v-if="license" style="margin-top:14px">
            <div v-if="license.licensed" class="alert ok">✔ 在授权范围内，适用合同 v{{ license.contract.version_no }}（{{ license.contract.effective_from }} 起）</div>
            <div v-else class="alert err">✘ {{ license.reason || '不在授权范围（越区/越语种/无合同）' }}</div>
            <template v-if="license.contract">
              <div class="kv small" style="margin-top:10px">
                <span class="k">授权区域</span><span>{{ license.contract.regions }}（本次查询区域{{ license.region_covered ? '已覆盖' : '未覆盖' }}）</span>
                <span class="k">授权语种</span><span>{{ license.contract.languages }}（本次查询语种{{ license.language_covered ? '已覆盖' : '未覆盖' }}）</span>
                <span class="k">纸书费率</span><span>{{ rateText(license.contract.print_rate_type, license.contract.print_rate) }}</span>
                <span class="k">电子书费率</span><span>{{ rateText(license.contract.ebook_rate_type, license.contract.ebook_rate) }}</span>
              </div>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, useAuth } from '../auth.js'

const props = defineProps({ id: String })
const auth = useAuth()
const isEditor = computed(() => auth.user.value.role === 'editor')
const data = ref(null)
const authors = ref([])
const err = ref('')
const okmsg = ref('')
const today = new Date().toISOString().slice(0, 10)
const q = ref({ date: today, region: 'JP', lang: 'en' })
const license = ref(null)
const cf = ref({
  effective_from: '', languages: 'zh,en', regions: 'CN,US',
  print_rate_type: 'percent', print_rate: 1000,
  ebook_rate_type: 'percent', ebook_rate: 1500,
  shares: [{ author_id: 0, share_bps: 10000 }], note: '',
})
const shareSum = computed(() => cf.value.shares.reduce((a, s) => a + (s.share_bps || 0), 0))

function rateText(type, v) {
  if (type === 'per_copy') return (v / 100).toFixed(2) + ' 元/册'
  return (v / 100).toFixed(2).replace(/\.?0+$/, '') + '%'
}
async function load() {
  data.value = await api(`/works/${props.id}`)
  authors.value = (await api('/authors')).items
}
async function submit() {
  err.value = ''; okmsg.value = ''
  try {
    await api(`/works/${props.id}/contracts`, { method: 'POST', body: JSON.stringify(cf.value) })
    okmsg.value = '新版本已保存，旧版本已按生效日自动截断。'
    cf.value.effective_from = ''
    await load()
  } catch (e) { err.value = e.message }
}
async function queryLicense() {
  const p = new URLSearchParams()
  if (q.value.date) p.set('date', q.value.date)
  if (q.value.region) p.set('region', q.value.region)
  if (q.value.lang) p.set('language', q.value.lang)
  license.value = await api(`/works/${props.id}/license?${p}`)
}
onMounted(load)
</script>
