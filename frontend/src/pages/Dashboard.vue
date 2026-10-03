<template>
  <div v-if="ov">
    <div class="grid3" style="margin-bottom:18px">
      <div class="stat">
        <div class="k">草稿结算单（可重算）</div>
        <div class="v">{{ ov.settlements.draft.count }} 张</div>
        <div class="small muted">{{ yuan(ov.settlements.draft.amount) }}</div>
      </div>
      <div class="stat">
        <div class="k">已确认·冻结</div>
        <div class="v">{{ ov.settlements.confirmed.count }} 张</div>
        <div class="small muted">{{ yuan(ov.settlements.confirmed.amount) }}</div>
      </div>
      <div class="stat">
        <div class="k">已付款</div>
        <div class="v">{{ ov.settlements.paid.count }} 张</div>
        <div class="small muted">{{ yuan(ov.settlements.paid.amount) }}</div>
      </div>
    </div>

    <div class="grid2">
      <div class="panel">
        <div class="panel-h">异常销量处理进度</div>
        <div class="panel-b">
          <div class="flex spread"><span>待处理</span><b class="t-neg" style="font-size:18px">{{ ov.exceptions.open }}</b></div>
          <div class="flex spread" style="margin-top:8px"><span>已处理（走补差单）</span><b style="font-size:18px;color:var(--ok)">{{ ov.exceptions.resolved }}</b></div>
          <div style="margin-top:14px"><router-link class="btn ghost sm" to="/sales/lines">去处理异常 →</router-link></div>
        </div>
      </div>
      <div class="panel">
        <div class="panel-h">补差单进度</div>
        <div class="panel-b">
          <div class="flex spread"><span>待确认/付款</span><b style="font-size:18px;color:var(--warn)">{‌{ ov.adjustments.pending.count }}</b></div>
          <div class="flex spread" style="margin-top:8px"><span>已付款补差（含负向）</span><b style="font-size:18px;color:var(--ok)">{{ yuan(ov.adjustments.paid.amount) }}</b></div>
          <div style="margin-top:14px"><router-link class="btn ghost sm" to="/adjustments">查看补差单 →</router-link></div>
        </div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-h">各结算期正常稿酬汇总</div>
      <div class="panel-b">
        <table>
          <thead><tr><th>结算期</th><th class="num">结算单数</th><th class="num">正常稿酬合计</th><th></th></tr></thead>
          <tbody>
            <tr v-for="r in ov.by_period" :key="r.period">
              <td class="mono"><b>{{ r.period }}</b></td>
              <td class="num">{{ r.count }}</td>
              <td class="num">{{ yuan(r.amount) }}</td>
              <td><router-link class="btn ghost sm" to="/settlements">打开该期结算单 →</router-link></td>
            </tr>
            <tr v-if="!ov.by_period.length"><td colspan="4" class="muted">暂无数据</td></tr>
      </tbody>
        </table>
        <div class="alert info small" style="margin-top:12px">
          幂等：共导入 {{ ov.batches.total }} 个批次，其中 <b>{{ ov.batches.duplicate_rejected }}</b> 个不同批次曾被重复导入并拦截（按批次号去重，不写入、不重复计酬）。
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../auth.js'
import { yuan } from '../format.js'
const ov = ref(null)
onMounted(async () => { ov.value = await api('/finance/overview') })
</script>
