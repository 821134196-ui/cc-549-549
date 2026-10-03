<template>
  <div>
    <div class="panel">
      <div class="panel-h">付款回执（本地模拟银行通道）</div>
      <div class="panel-b">
        <table>
          <thead><tr><th>回执号</th><th>付款对象</th><th class="num">金额</th><th>通道</th><th>付款时间</th><th>收款信息</th></tr></thead>
          <tbody>
            <tr v-for="r in list" :key="r.id">
              <td class="mono small"><b>{{ r.receipt_no }}</b></td>
              <td class="small">{{ r.payable_type === 'settlement' ? '结算单' : '补差单' }} #{{ r.payable_id }}</td>
              <td class="num">{{ yuan(r.amount) }}</td>
              <td class="small">{{ r.channel }}</td>
              <td class="small muted">{{ r.paid_at?.replace('T',' ').slice(0,19) }}</td>
              <td class="small muted">{{ r.memo }}</td>
            </tr>
            <tr v-if="!list.length"><td colspan="6" class="muted">暂无回执</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../auth.js'
import { yuan } from '../format.js'
const list = ref([])
onMounted(async () => { list.value = (await api('/payments')).items })
</script>
