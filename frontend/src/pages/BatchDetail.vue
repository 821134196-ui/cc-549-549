<template>
  <div v-if="batch">
    <div class="flex" style="margin-bottom:14px">
      <router-link to="/batches" class="muted small">← 返回批次列表</router-link>
    </div>
    <div class="panel">
      <div class="panel-h">
        <div>批次 <b class="mono">{{ batch.batch_ref }}</b> · {{ batch.period }}</div>
        <span v-if="batch.duplicate_hits" class="badge ex">被重复导入 {{ batch.duplicate_hits }} 次（均已拦截）</span>
      </div>
      <div class="panel-b">
        <p class="small muted mt0">备注：{{ batch.note }}　|　原始渠道单据号在“单据”列，可逐行追溯。</p>
        <table>
          <thead><tr>
            <th>#</th><th>作品</th><th>格式</th><th>语种</th><th>区域</th><th>销售日</th>
            <th class="num">数量</th><th class="num">单价</th><th class="num">金额</th>
            <th>单据 / 退货对应</th><th class="num">版税估算</th><th>判定</th>
          </tr></thead>
          <tbody>
            <tr v-for="(l, i) in batch.lines" :key="l.id">
              <td>{{ i + 1 }}</td>
              <td>#{{ l.work_id }}</td>
              <td>{{ l.format === 'print' ? '纸书' : '电子书' }}</td>
              <td>{{ l.language }}</td>
              <td><b>{{ l.region }}</b></td>
              <td class="small">{{ l.sale_date }}</td>
              <td class="num" :class="{ 't-neg': l.qty < 0 }">{{ l.qty }}</td>
              <td class="num">{{ yuan(l.unit_price) }}</td>
              <td class="num" :class="{ 't-neg': l.amount < 0 }">{{ yuan(l.amount) }}</td>
              <td class="small mono">{{ l.doc_ref }}<span v-if="l.return_ref"> ←退 {{ l.return_ref }}</span></td>
              <td class="num" :class="{ 't-neg': l.royalty_gross < 0 }">{{ yuan(l.royalty_gross) }}</td>
              <td>
                <span v-if="l.status === 'exception'" class="badge ex">{{ EXCEPTION_LABELS[l.exception_type]?.label || l.exception_type }}</span>
                <span v-else class="badge ok">{{ l.status === 'settled' ? '已计酬' : '正常' }}</span>
                <div v-if="l.status==='exception'" class="small muted" style="max-width:260px">{{ l.exception_memo }}</div>
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
import { yuan, EXCEPTION_LABELS } from '../format.js'
const props = defineProps({ id: String })
const batch = ref(null)
onMounted(async () => { batch.value = await api(`/batches/${props.id}`) })
</script>
