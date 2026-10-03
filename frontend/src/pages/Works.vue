<template>
  <div>
    <div class="panel" v-if="isEditor">
      <div class="panel-h">新建作品</div>
      <div class="panel-b">
        <div class="row">
          <div><label>书名 *</label><input v-model="form.title" /></div>
          <div><label>ISBN</label><input v-model="form.isbn" placeholder="978-..." /></div>
        </div>
        <label>简介</label><input v-model="form.description" />
        <div style="margin-top:12px"><button class="btn" @click="create">创建作品</button></div>
        <div v-if="err" class="alert err">{{ err }}</div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-h">
        作品列表
        <input v-model="kw" placeholder="搜索书名 / ISBN" style="width:240px" @input="reload" />
      </div>
      <div class="panel-b">
        <table>
          <thead><tr><th>ID</th><th>书名</th><th>ISBN</th><th>合同版本</th><th>授权范围</th><th></th></tr></thead>
          <tbody>
            <tr v-for="w in works" :key="w.id">
              <td>{{ w.id }}</td>
              <td><b>{{ w.title }}</b><div class="small muted">{{ w.description }}</div></td>
              <td class="mono small">{{ w.isbn }}</td>
              <td>{{ versionCount[w.id] || 0 }} 版</td>
              <td class="small">{{ scopeText[w.id] }}</td>
              <td><router-link class="btn ghost sm" :to="`/works/${w.id}`">查看 / 维护合同</router-link></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { api, useAuth } from '../auth.js'

const auth = useAuth()
const isEditor = computed(() => auth.user.value.role === 'editor')
const works = ref([])
const kw = ref('')
const form = ref({ title: '', isbn: '', description: '' })
const err = ref('')
const details = ref({})
const versionCount = computed(() => {
  const m = {}
  for (const d of Object.values(details.value)) m[d.work.id] = d.versions.length
  return m
})
const scopeText = computed(() => {
  const m = {}
  for (const d of Object.values(details.value)) {
    const v = d.versions[0]
    m[d.work.id] = v ? `${v.regions} · ${v.languages} · v${v.version_no} 生效 ${v.effective_from}` : '尚无合同'
  }
  return m
})

let timer
function reload() {
  clearTimeout(timer)
  timer = setTimeout(load, 250)
}
async function load() {
  const r = await api('/works' + (kw.value ? `?q=${encodeURIComponent(kw.value)}` : ''))
  works.value = r.items
  await Promise.all(works.value.map(async (w) => {
    if (!details.value[w.id]) details.value[w.id] = await api(`/works/${w.id}`)
  }))
}
async function create() {
  err.value = ''
  if (!form.value.title) { err.value = '请填写书名'; return }
  try {
    await api('/works', { method: 'POST', body: JSON.stringify(form.value) })
    form.value = { title: '', isbn: '', description: '' }
    await load()
  } catch (e) { err.value = e.message }
}
onMounted(load)
</script>
