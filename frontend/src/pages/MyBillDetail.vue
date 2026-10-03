<template>
  <div v-if="s">
    <div class="flex" style="margin-bottom:14px">
      <router-link to="/my/bills" class="muted small">← 返回我的账单</router-link>
    </div>

    <div class="panel">
      <div class="panel-h">
        <div>结算单 #{{ s.id }} · <span class="mono">{{ s.period }}</span></div>
        <span class="badge" :class="STATUS_LABELS[s.status].cls">{{ STATUS_LABELS[s.status].label }}</span>
      </div>
      <div class="panel-b">
        <div class="grid3">
          <div class="stat"><div class="k">正常稿酬</div><div class="v">{{ yuan(s.normal_amount) }}</div></div>
          <div class="stat"><div class="k">补差合计</div><div class="v" :class="s.adjustment_total < 0 ? 't-neg' : ''">{{ yuan(s.adjustment_total) }}</div></div>
          <div class="stat"><div class="k">最终应付</div><div class="v">{{ yuan(s.payable_total) }}</div></div>
        </div>
        <div v-if="s.receipt" class="alert ok small" style="margin-top:12px">
          付款回执 <b class="mono">{{ s.receipt.receipt_no }}</b>：{{ yuan(s.receipt.amount) }}，{{ s.paid_at?.replace('T',' ').slice(0,19) }}
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-h">我的计算明细（每条来自哪笔原始销量、按哪个合同版本计算）</div>
      <div class="panel-b">
        <table>
          <thead><tr>
            <th>作品</th><th>销售日</th><th>区域</th><th>格式</th><th>类型</th>
            <th class="num">数量</th><th class="num">原始销售额</th><th>合同依据</th>
            <th class="num">版税总额</th><th class="num">我的比例</th><th class="num">我的稿酬</th><th>原始行</th>
          </tr></thead>
          <tbody>
            <tr v-for="e in s.entries" :key="e.id">
              <td>#{{ e.work_id }}</td>
              <td class="small">{{ e.sale_date }}</td>
              <td>{{ e.region }}</td>
              <td>{{ e.format === 'print' ? '纸书' : '电子书' }}</td>
              <td><span v-if="e.kind==='return'" class="badge ex">退货</span><span v-else class="badge ok">销售</span></td>
              <td class="num" :class="{ 't-neg': e.qty < 0 }">{{ e.qty }}</td>
              <td class="num">{{ yuan(e.amount) }}</td>
              <td class="small">合同 #{{ e.contract_id }}</td>
              <td class="num">{{ yuan(e.royalty_gross) }}</td>
              <td class="num">{{ (e.share_bps/100).toFixed(2).replace(/\.?0+$/,'') }}%</td>
              <td class="num" :class="{ 't-neg': e.royalty < 0 }"><b>{{ yuan(e.royalty) }}</b></td>
              <td class="small mono">#{{ e.sales_line_id }}</td>
            </tr>
            <tr class="total-row">
              <td colspan="10">合计</td>
              <td class="num">{{ yuan(s.normal_amount) }}</td><td></td>
            </tr>
          </tbody>
        </table>
        <p class="small muted" style="margin-bottom:0">
          计酬严格采用<b>每笔销售发生当日有效</b>的合同版本与费率；您的分成比例与原始销量逐行对应，如有疑问可凭“原始行”编号与出版社核对，无需邮件往返。
        </p>
      </div>
    </div>

    <div class="panel" v-if="s.related_adjustments.length">
      <div class="panel-h">与本单相关的补差单</div>
      <div class="panel-b">
        <table>
          <thead><tr><th>单号</th><th>原因</th><th>说明</th><th class="num">金额</th><th>状态</th></tr></thead>
          <tbody>
            <tr v-for="a in s.related_adjustments" :key="a.id">
              <td class="mono small">{{ a.code }}</td>
              <td class="small">{{ a.reason }}</td>
              <td class="small muted">{{ a.memo }}</td>
              <td class="num" :class="{ 't-neg': a.amount < 0 }">{{ yuan(a.amount) }}</td>
              <td><span class="badge" :class="STATUS_LABELS[a.status]?.cls">{{ STATUS_LABELS[a.status]?.label || a.status }}</span></td>
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
const props = defineProps({ id: String })
const s = ref(null)
onMounted(async () => { s.value = await api(`/my/settlements/${props.id}`) })
</script>
