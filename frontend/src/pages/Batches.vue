<template>
  <div>
    <div class="panel">
      <div class="panel-h">
        导入批次
        <input v-model="period" placeholder="按期筛选 2026-08" style="width:200px" @change="load" />
      </div>
      <div class="panel-b">
        <table>
          <thead><tr><th>ID</th><th>批次号</th><th>归属期</th><th>行数</th><th>重复导入拦截次数</th><th>导入时间</th><th>备注</th><th></th></tr></thead>
          <tbody>
            <tr v-for="b in batches" :key="b.id">
              <td>{{ b.id }}</td>
              <td class="mono"><b>{{ b.batch_ref }}</b></td>
              <td>{{ b.period }}</td>
              <td>{{ b.row_count }}</td>
              <td>
                <span v-if="b.duplicate_hits" class="badge ex">{{ b.duplicate_hits }} 次</span>
                <span v-else class="muted">0</span>
              </td>
              <td class="small muted">{{ (b.imported_at||'').replace('T',' ').slice(0,19) }}</td>
              <td class="small muted">{{ b.note }}</td>
              <td><router-link class="btn ghost sm" :to="`/batches/${b.id}`">查看明细</router-link></td>
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
const batches = ref([])
const period = ref('')
async function load() {
  batches.value = (await api('/batches' + (period.value ? `?period=${period.value}` : ''))).items
}
onMounted(load)
</script>
