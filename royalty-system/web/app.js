const { createApp, reactive, computed, onMounted } = Vue;

// 全局提示总线（子组件直接调用 notify）
const bus = reactive({ type: '', msg: '' });
let busTimer = null;
function notify(type, msg) {
  bus.type = type; bus.msg = msg;
  clearTimeout(busTimer);
  busTimer = setTimeout(() => (bus.msg = ''), 4200);
}

// ---------- 工具 ----------
const yuan = (cents) => {
  if (cents === null || cents === undefined) return '-';
  const neg = cents < 0;
  const v = Math.abs(cents);
  const s = (Math.floor(v / 100) + '.' + String(v % 100).padStart(2, '0'));
  return (neg ? '-' : '') + '¥' + s;
};
const CAT_LABEL = {
  normal: '正常计酬',
  cross_region: '跨区/超授权',
  cross_period_return: '跨期退货',
  no_contract: '无有效合同',
};
const STATUS_LABEL = { draft: '草稿', confirmed: '已确认', paid: '已付款' };
const ROLE_LABEL = { editor: '编辑', finance: '财务', author: '作者' };
const REGIONS = [
  { code: 'CN', name: '中国大陆' }, { code: 'TW', name: '中国台湾' },
  { code: 'HK', name: '中国香港' }, { code: 'MO', name: '中国澳门' },
  { code: 'US', name: '美国' }, { code: 'JP', name: '日本' }, { code: 'OTHER', name: '其他' },
];
const LANGS = [
  { code: 'zh', name: '简体中文' }, { code: 'zht', name: '繁体中文' },
  { code: 'en', name: '英文' }, { code: 'ja', name: '日文' },
];
const regionName = (c) => (REGIONS.find(r => r.code === c) || {}).name || c;
const langName = (c) => (LANGS.find(l => l.code === c) || {}).name || c;
const bpsToPct = (b) => (b / 100).toFixed(2).replace(/\.?0+$/, '') + '%';
const pctToBps = (p) => Math.round(parseFloat(p) * 100);

const api = {
  token: localStorage.getItem('royalty_token') || '',
  user: JSON.parse(localStorage.getItem('royalty_user') || 'null'),
  async call(method, path, body) {
    const opt = { method, headers: {} };
    if (this.token) opt.headers.Authorization = 'Bearer ' + this.token;
    if (body instanceof FormData) { opt.body = body; }
    else if (body !== undefined) { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
    const res = await fetch('/api' + path, opt);
    const data = await res.json().catch(() => ({}));
    if (res.status === 401) { if (api.on401) api.on401(); throw new Error(data.error || '未登录'); }
    if (!res.ok) throw new Error(data.error || ('请求失败 ' + res.status));
    return data;
  },
  save(user, token) {
    this.user = user; this.token = token;
    localStorage.setItem('royalty_token', token);
    localStorage.setItem('royalty_user', JSON.stringify(user));
  },
  clear() {
    this.user = null; this.token = '';
    localStorage.removeItem('royalty_token');
    localStorage.removeItem('royalty_user');
  },
};

// ---------- 根组件 ----------
const App = {
  setup() {
    const st = reactive({
      tab: 'main',
      works: [], authors: [],
    });
    api.on401 = () => { api.clear(); location.reload(); };
    const logout = () => { api.clear(); location.reload(); };

    const defaultTab = () => (api.user.role === 'editor' ? 'works' : api.user.role === 'finance' ? 'settlements' : 'mine');

    onMounted(async () => {
      st.tab = defaultTab();
      if (api.user) {
        try {
          st.authors = await api.call('GET', '/authors');
          st.works = await api.call('GET', '/works');
        } catch (e) { /* 忽略，具体页面提示 */ }
      }
    });

    return { st, bus, logout,
      get user() { return api.user; },
      setTab(t) { st.tab = t; },
    };
  },
  template: `
  <template v-if="!user">
    <login-view />
  </template>
  <template v-else>
    <div class="topbar">
      <div class="brand">📚 稿酬结算系统</div>
      <div class="tabs">
        <template v-if="user.role==='editor'">
          <div class="tab" :class="{active: st.tab==='works'}" @click="setTab('works')">作品与合同</div>
        </template>
        <template v-if="user.role==='finance'">
          <div class="tab" :class="{active: st.tab==='settlements'}" @click="setTab('settlements')">结算单</div>
          <div class="tab" :class="{active: st.tab==='import'}" @click="setTab('import')">销量导入</div>
          <div class="tab" :class="{active: st.tab==='exceptions'}" @click="setTab('exceptions')">异常销量</div>
        </template>
        <template v-if="user.role==='author'">
          <div class="tab" :class="{active: st.tab==='mine'}" @click="setTab('mine')">我的稿酬</div>
          <div class="tab" :class="{active: st.tab==='exceptions'}" @click="setTab('exceptions')">异常项</div>
        </template>
      </div>
      <div class="userbox"><b>{{user.name}}</b> · {{ {editor:'编辑',finance:'财务',author:'作者'}[user.role] }}</div>
      <button @click="logout">退出</button>
    </div>
    <div class="container">
      <div v-if="bus.msg" class="flash" :class="bus.type">{{ bus.msg }}</div>
      <works-view v-if="user.role==='editor' && st.tab==='works'" />
      <settlements-view v-if="user.role==='finance' && st.tab==='settlements'" scope="all" />
      <import-view v-if="user.role==='finance' && st.tab==='import'" />
      <exceptions-view v-if="(user.role==='finance'||user.role==='author') && st.tab==='exceptions'" />
      <settlements-view v-if="user.role==='author' && st.tab==='mine'" scope="mine" />
    </div>
  </template>`,
};

// ---------- 登录 ----------
App.components = {};
const LoginView = {
  setup() {
    const f = reactive({ login_name: '', password: '', err: '', loading: false });
    const submit = async () => {
      f.err = ''; f.loading = true;
      try {
        const r = await api.call('POST', '/login', { login_name: f.login_name, password: f.password });
        api.save(r.user, r.token);
        location.reload();
      } catch (e) { f.err = e.message; } finally { f.loading = false; }
    };
    const fill = (n) => { f.login_name = n; f.password = n === 'editor' ? 'editor123' : n === 'finance' ? 'finance123' : 'author123'; };
    return { f, submit, fill };
  },
  template: `
  <div class="login-wrap">
    <div class="card">
      <h1>📚 稿酬结算与授权区域系统</h1>
      <div class="muted small mb8">出版社内部系统 · 编辑 / 财务 / 作者</div>
      <div v-if="f.err" class="flash err">{{f.err}}</div>
      <label>登录账号</label>
      <input v-model="f.login_name" @keyup.enter="submit" placeholder="editor / finance / wangming" />
      <label>密码</label>
      <input type="password" v-model="f.password" @keyup.enter="submit" />
      <button class="primary mt16" style="width:100%" :disabled="f.loading" @click="submit">{{f.loading ? '登录中…' : '登 录'}}</button>
      <div class="demo-accounts">
        演示账号（点击自动填充）：<br/>
        <a @click="fill('editor')"><code>editor / editor123</code></a> 编辑：维护作品合同<br/>
        <a @click="fill('finance')"><code>finance / finance123</code></a> 财务：导入销量、结算付款<br/>
        <a @click="fill('wangming')"><code>wangming / author123</code></a> 作者王明（分成60%）<br/>
        <a @click="fill('lihua')"><code>lihua / author123</code></a> 作者李华（分成40%）
      </div>
    </div>
  </div>`,
};

// ---------- 编辑：作品与合同 ----------
const WorksView = {
  setup() {
    const s = reactive({
      works: [], authors: [], selectedId: null, detail: null, authDate: '',
      showWork: false, newWork: { code: '', title: '', isbn: '' },
      showContract: false,
      contract: { effective_from: '', effective_until: '', print: '10', ebook: '15', note: '', participants: [{ author_id: null, share: '' }], regions: [], languages: [] },
    });
    const load = async () => {
      s.works = await api.call('GET', '/works');
      s.authors = await api.call('GET', '/authors');
      if (!s.selectedId && s.works.length) s.selectedId = s.works[0].id;
      if (s.selectedId) await select(s.selectedId);
    };
    const select = async (id) => {
      s.selectedId = id;
      s.detail = await api.call('GET', `/works/${id}/authorization`);
    };
    const queryDate = async () => {
      const q = s.authDate ? '?date=' + s.authDate : '';
      s.detail = await api.call('GET', `/works/${s.selectedId}/authorization${q}`);
    };
    const addWork = async () => {
      try {
        await api.call('POST', '/works', s.newWork);
        s.showWork = false; s.newWork = { code: '', title: '', isbn: '' };
        notify('ok', '作品已创建');
        await load();
      } catch (e) { notify('err', e.message); }
    };
    const addParticipant = () => s.contract.participants.push({ author_id: null, share: '' });
    const delParticipant = (i) => s.contract.participants.splice(i, 1);
    const toggle = (arr, v) => {
      const i = arr.indexOf(v);
      if (i >= 0) arr.splice(i, 1); else arr.push(v);
    };
    const shareSum = computed(() => s.contract.participants.reduce((a, p) => a + (parseFloat(p.share) || 0), 0));
    const saveContract = async () => {
      const body = {
        effective_from: s.contract.effective_from,
        effective_until: s.contract.effective_until,
        print_rate_bps: pctToBps(s.contract.print),
        ebook_rate_bps: pctToBps(s.contract.ebook),
        note: s.contract.note,
        participants: s.contract.participants
          .filter(p => p.author_id)
          .map(p => ({ author_id: Number(p.author_id), share_bps: pctToBps(p.share) })),
        regions: s.contract.regions, languages: s.contract.languages,
      };
      try {
        await api.call('POST', `/works/${s.selectedId}/contracts`, body);
        notify('ok', '合同新版本已创建');
        s.showContract = false;
        await select(s.selectedId);
      } catch (e) { notify('err', e.message); }
    };
    onMounted(load);
    return { s, load, select, queryDate, addWork, addParticipant, delParticipant, toggle, shareSum, saveContract,
      REGIONS, LANGS, yuan, bpsToPct, regionName, langName };
  },
  template: `
  <div class="grid2" style="grid-template-columns: 260px 1fr; align-items:start;">
    <div class="card">
      <div class="flex-between mb8"><h3 style="margin:0">作品</h3><button @click="s.showWork=true">+ 新建</button></div>
      <div v-for="w in s.works" :key="w.id" class="tab" :class="{active: s.selectedId===w.id}" @click="select(w.id)" style="display:block;margin-bottom:4px">
        {{w.title}} <div class="small muted">{{w.code}} · {{w.isbn}}</div>
      </div>
    </div>

    <div v-if="s.detail">
      <div class="card">
        <div class="flex-between">
          <div><h3 style="margin:0">{{s.detail.work.title}}</h3>
            <div class="muted small">{{s.detail.work.code}} · ISBN {{s.detail.work.isbn}}</div></div>
          <button class="primary" @click="s.showContract=true">+ 新增合同版本</button>
        </div>
      </div>

      <div class="card">
        <h3>授权范围查询</h3>
        <div class="flex gap8">
          <div style="width:180px"><input type="date" v-model="s.authDate" /></div>
          <button @click="queryDate">查询生效合同</button>
          <span class="muted small">当前查询日期：{{s.detail.query_date}}</span>
        </div>
        <div v-if="s.detail.effective" class="mt12 reason-box">
          <b>该日生效：合同 v{{s.detail.effective.version_no}}</b>
          （{{s.detail.effective.effective_from.slice(0,10)}}
          ~ {{s.detail.effective.effective_until ? s.detail.effective.effective_until.slice(0,10) : '长期'}}）<br/>
          授权区域：<span class="tag" v-for="r in s.detail.effective.regions" :key="r.id">{{regionName(r.region)}}</span><br/>
          授权语种：<span class="tag" v-for="l in s.detail.effective.languages" :key="l.id">{{langName(l.language)}}</span><br/>
          费率：纸书 {{bpsToPct(s.detail.effective.print_rate_bps)}} / 电子书 {{bpsToPct(s.detail.effective.ebook_rate_bps)}}
        </div>
        <div v-else class="mt12 flash err" style="margin-bottom:0">该日期没有生效合同（到期后销量将归入异常，不计稿酬）</div>
      </div>

      <div class="card">
        <h3>合同版本历史</h3>
        <table>
          <thead><tr><th>版本</th><th>有效期</th><th>纸书费率</th><th>电子书费率</th><th>作者分成</th><th>区域 / 语种</th><th>备注</th></tr></thead>
          <tbody>
          <tr v-for="v in s.detail.versions" :key="v.id">
            <td><b>v{{v.version_no}}</b></td>
            <td class="small">{{v.effective_from.slice(0,10)}}<br/>~ {{v.effective_until ? v.effective_until.slice(0,10) : '长期'}}</td>
            <td>{{bpsToPct(v.print_rate_bps)}}</td>
            <td>{{bpsToPct(v.ebook_rate_bps)}}</td>
            <td class="small">
              <div v-for="p in v.participants" :key="p.id">
                {{ (s.authors.find(a=>a.id===p.author_id)||{}).name || ('作者#'+p.author_id) }}：{{bpsToPct(p.share_bps)}}
              </div>
            </td>
            <td class="small">
              <span class="tag" v-for="r in v.regions" :key="r.id">{{regionName(r.region)}}</span>
              <span class="tag" v-for="l in v.languages" :key="l.id">{{langName(l.language)}}</span>
            </td>
            <td class="small muted">{{v.note}}</td>
          </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <!-- 新建作品 -->
  <div v-if="s.showWork" class="modal-mask" @click.self="s.showWork=false">
    <div class="modal" style="max-width:440px">
      <h3>新建作品</h3>
      <label>作品编号 *</label><input v-model="s.newWork.code" placeholder="W002" />
      <label>书名 *</label><input v-model="s.newWork.title" />
      <label>ISBN</label><input v-model="s.newWork.isbn" />
      <div class="mt16 flex" style="justify-content:flex-end;gap:8px">
        <button @click="s.showWork=false">取消</button>
        <button class="primary" @click="addWork">创建</button>
      </div>
    </div>
  </div>

  <!-- 新增合同版本 -->
  <div v-if="s.showContract" class="modal-mask" @click.self="s.showContract=false">
    <div class="modal">
      <h3>新增合同版本（按生效日期自动判定适用版本）</h3>
      <div class="grid2">
        <div><label>生效日 *</label><input type="date" v-model="s.contract.effective_from" /></div>
        <div><label>失效日（空=长期）</label><input type="date" v-model="s.contract.effective_until" /></div>
        <div><label>纸书费率 %</label><input v-model="s.contract.print" placeholder="10" /></div>
        <div><label>电子书费率 %</label><input v-model="s.contract.ebook" placeholder="15" /></div>
      </div>
      <label>备注</label><input v-model="s.contract.note" />

      <h4>作者分成（合计须 100%，当前 {{shareSum.toFixed(2)}}%）</h4>
      <div v-for="(p,i) in s.contract.participants" :key="i" class="row-author">
        <select v-model="p.author_id">
          <option :value="null">选择作者</option>
          <option v-for="a in s.authors" :key="a.id" :value="a.id">{{a.name}}（{{a.code}}）</option>
        </select>
        <input v-model="p.share" placeholder="分成 %，如 60" />
        <button class="danger" @click="delParticipant(i)" v-if="s.contract.participants.length>1">×</button>
      </div>
      <button @click="addParticipant">+ 添加作者</button>

      <h4>授权区域</h4>
      <label class="checkline" v-for="r in REGIONS" :key="r.code">
        <input type="checkbox" :checked="s.contract.regions.includes(r.code)" @change="toggle(s.contract.regions, r.code)" /> {{r.name}}（{{r.code}}）
      </label>
      <h4>授权语种</h4>
      <label class="checkline" v-for="l in LANGS" :key="l.code">
        <input type="checkbox" :checked="s.contract.languages.includes(l.code)" @change="toggle(s.contract.languages, l.code)" /> {{l.name}}（{{l.code}}）
      </label>

      <div class="mt16 flex" style="justify-content:flex-end;gap:8px">
        <button @click="s.showContract=false">取消</button>
        <button class="primary" @click="saveContract">保存合同版本</button>
      </div>
    </div>
  </div>`,
};

// ---------- 财务：结算单列表 + 详情 ----------
const SettlementsView = {
  props: ['scope'],
  setup() {
    const s = reactive({ list: [], names: {}, period: '', status: '', loading: false, detail: null, adjust: { show: false, amount: '', reason: '' } });
    const load = async () => {
      s.loading = true;
      try {
        const q = new URLSearchParams();
        if (s.period) q.set('period', s.period);
        if (s.status) q.set('status', s.status);
        const r = await api.call('GET', '/settlements?' + q.toString());
        s.list = r.settlements; s.names = r.author_names;
      } finally { s.loading = false; }
    };
    const open = async (id) => { s.detail = await api.call('GET', '/settlements/' + id); };
    const close = () => { s.detail = null; load(); };
    const confirm = async (d) => {
      await api.call('POST', `/settlements/${d.id}/confirm`);
      notify('ok', '结算单已确认，金额锁定');
      await open(d.id); await load();
    };
    const pay = async (d) => {
      const r = await api.call('POST', `/settlements/${d.id}/pay`);
      notify('ok', '已生成模拟付款回执：' + r.receipt.ref_code);
      await open(d.id); await load();
    };
    const regen = async () => {
      const r = await api.call('POST', '/settlements/regenerate');
      notify('info', `重算完成：扫描 ${r.scanned_records} 条销量，生成 ${r.generated_lines} 条作者明细`);
      await load();
    };
    const submitAdjust = async () => {
      const cents = Math.round(parseFloat(s.adjust.amount) * 100);
      try {
        await api.call('POST', `/settlements/${s.detail.id}/adjust`, { amount_cents: cents, reason: s.adjust.reason });
        notify('ok', '补差单已开具并关联本结算单');
        s.adjust.show = false; s.adjust.amount = ''; s.adjust.reason = '';
        await open(s.detail.id);
      } catch (e) { notify('err', e.message); }
    };
    onMounted(load);
    return { s, load, open, close, confirm, pay, regen, submitAdjust, yuan, CAT_LABEL, STATUS_LABEL };
  },
  template: `
  <div class="card">
    <div class="flex-between mb8">
      <h3 style="margin:0">{{scope==='mine' ? '我的稿酬结算单' : '结算单管理'}}</h3>
      <button v-if="scope!=='mine'" @click="regen">↻ 重新计算草稿单</button>
    </div>
    <div class="toolbar">
      <div><label>期号</label><input placeholder="2025-06" v-model="s.period" /></div>
      <div><label>状态</label>
        <select v-model="s.status"><option value="">全部</option><option value="draft">草稿</option><option value="confirmed">已确认</option><option value="paid">已付款</option></select>
      </div>
      <button @click="load">筛选</button>
    </div>
    <table>
      <thead><tr><th>期号</th><th v-if="scope!=='mine'">作者</th><th>类型</th><th>状态</th><th class="num">正常应付</th><th class="num">异常隔离</th><th></th></tr></thead>
      <tbody>
      <tr v-for="st in s.list" :key="st.id" class="clickable" @click="open(st.id)">
        <td class="mono">{{st.period}}</td>
        <td v-if="scope!=='mine'">{{s.names[st.author_id] || ('#'+st.author_id)}}</td>
        <td><span class="badge" :class="st.kind==='supplement'?'supplement':'kind-normal'">{{st.kind==='supplement'?'补结算单':'正常结算'}}</span></td>
        <td><span class="badge" :class="st.status">{{STATUS_LABEL[st.status]}}</span></td>
        <td class="num amount-normal">{{yuan(st.normal_amount_cents)}}</td>
        <td class="num amount-ex">{{yuan(st.exception_amount_cents)}}</td>
        <td class="num"><a @click.stop="open(st.id)">详情 →</a></td>
      </tr>
      <tr v-if="!s.list.length"><td colspan="7" class="muted" style="text-align:center;padding:24px">暂无结算单</td></tr>
      </tbody>
    </table>
  </div>

  <!-- 详情弹窗 -->
  <div v-if="s.detail" class="modal-mask">
    <div class="modal" style="max-width:980px">
      <div class="flex-between">
        <h3>结算单 #{{s.detail.id}} · {{s.detail.author.name}} · {{s.detail.period}}
          <span class="badge" :class="s.detail.status" style="margin-left:8px">{{STATUS_LABEL[s.detail.status]}}</span>
          <span v-if="s.detail.kind==='supplement'" class="badge supplement">补结算单</span>
        </h3>
        <button @click="close">关闭</button>
      </div>
      <div v-if="s.detail.linked_settlement" class="draft-note mt12">
        本单为补结算单，关联原结算单 #{{s.detail.linked_settlement.id}}（{{s.detail.linked_settlement.period}}，
        状态：{{STATUS_LABEL[s.detail.linked_settlement.status]}}）。{{s.detail.note}}
      </div>

      <div class="totals-bar mt12">
        <div><div class="stat-label">正常稿酬应付</div><b class="amount-normal">{{yuan(s.detail.normal_amount_cents)}}</b></div>
        <div><div class="stat-label">异常项（隔离不计付）</div><b class="amount-ex">{{yuan(s.detail.exception_amount_cents)}}</b></div>
        <div><div class="stat-label">补差累计</div><b>{{yuan(s.detail.adjustments.reduce((a,x)=>a+x.amount_cents,0))}}</b></div>
        <div><div class="stat-label">实际应付合计</div><b style="color:var(--brand)">{{yuan(s.detail.payable_cents)}}</b></div>
      </div>

      <div class="flex mb8" v-if="scope!=='mine'">
        <button class="primary" v-if="s.detail.status==='draft'" @click="confirm(s.detail)">✓ 确认结算单（金额锁定）</button>
        <button v-if="s.detail.status==='confirmed'" @click="s.adjust.show=true">+ 开补差单</button>
        <button class="primary" v-if="s.detail.status==='confirmed'" @click="pay(s.detail)">💳 模拟付款</button>
        <span v-if="s.detail.status==='draft'" class="small muted">草稿单会随新销量导入自动重算；确认后只能补差，不能改金额</span>
      </div>

      <h4>结算明细（可追溯原始销量与抵扣依据）</h4>
      <table>
        <thead><tr><th>分类</th><th>作品</th><th>销售日</th><th>渠道行号</th><th>区域/语种</th><th>类型</th><th class="num">原始金额</th><th class="num">费率</th><th class="num">分成</th><th class="num">应付</th><th>依据/原因</th></tr></thead>
        <tbody>
        <tr v-for="ln in s.detail.lines" :key="ln.id">
          <td><span class="badge" :class="ln.category">{{CAT_LABEL[ln.category]}}</span></td>
          <td class="small">{{s.detail.work_titles[ln.work_id]}}</td>
          <td class="small mono">{{ln.sale_date.slice(0,10)}}</td>
          <td class="small mono">
            {{s.detail.sales_refs[ln.sales_record_id]?.ref}}
            <div v-if="s.detail.sales_refs[ln.sales_record_id]?.original_ref" class="muted">
              抵扣原销售 {{s.detail.sales_refs[ln.sales_record_id].original_ref}}
            </div>
            <div class="muted">数量 {{s.detail.sales_refs[ln.sales_record_id]?.qty}} · 批次#{{s.detail.sales_refs[ln.sales_record_id]?.batch_id}}</div>
          </td>
          <td class="small">{{ln.region}}/{{ln.language}}</td>
          <td class="small">{{ln.format==='print'?'纸书':'电子书'}}</td>
          <td class="num" :class="{'amount-neg':ln.base_amount_cents<0}">{{yuan(ln.base_amount_cents)}}</td>
          <td class="num">{{(ln.rate_bps/100).toFixed(1).replace(/\.0$/,'')}}%</td>
          <td class="num">{{(ln.share_bps/100).toFixed(0)}}%</td>
          <td class="num" :class="{'amount-neg':ln.amount_cents<0}"><b>{{yuan(ln.amount_cents)}}</b></td>
          <td class="small muted" style="max-width:220px">{{ln.reason}}<div v-if="ln.contract_version_id">合同版本 #{{ln.contract_version_id}}</div></td>
        </tr>
        </tbody>
      </table>

      <h4 v-if="s.detail.adjustments.length">补差单</h4>
      <table v-if="s.detail.adjustments.length">
        <thead><tr><th>#</th><th class="num">金额</th><th>原因</th></tr></thead>
        <tbody><tr v-for="a in s.detail.adjustments" :key="a.id">
          <td>{{a.id}}</td><td class="num" :class="{'amount-neg':a.amount_cents<0}"><b>{{yuan(a.amount_cents)}}</b></td>
          <td class="small">{{a.reason}}</td></tr></tbody>
      </table>

      <div v-if="s.detail.receipt" class="mt12 reason-box">
        💳 付款回执：<b>{{s.detail.receipt.ref_code}}</b> · 金额 {{yuan(s.detail.receipt.amount_cents)}}
        · {{s.detail.receipt.note}} · {{s.detail.receipt.created_at.slice(0,16).replace('T',' ')}}
      </div>
    </div>
  </div>

  <!-- 补差单弹窗 -->
  <div v-if="s.adjust.show" class="modal-mask" @click.self="s.adjust.show=false">
    <div class="modal" style="max-width:460px">
      <h3>对结算单 #{{s.detail?.id}} 开补差单</h3>
      <div class="muted small">已确认/已付款结算单不得改金额，差错通过补差单更正（负数为冲减）。</div>
      <label>补差金额（元）</label><input v-model="s.adjust.amount" placeholder="5.00 或 -12.00" />
      <label>原因 *</label><textarea v-model="s.adjust.reason" rows="3" placeholder="如：6月少计渠道补贴"></textarea>
      <div class="mt16 flex" style="justify-content:flex-end;gap:8px">
        <button @click="s.adjust.show=false">取消</button>
        <button class="primary" @click="submitAdjust">开具补差单</button>
      </div>
    </div>
  </div>`,
};

// ---------- 财务：销量导入 ----------
const ImportView = {
  setup() {
    const s = reactive({ file: null, result: null, err: '', busy: false, batches: [] });
    const loadBatches = async () => { s.batches = await api.call('GET', '/batches'); };
    const pick = (e) => { s.file = e.target.files[0]; s.result = null; s.err = ''; };
    const doImport = async () => {
      if (!s.file) { s.err = '请先选择 CSV 文件'; return; }
      s.busy = true; s.err = '';
      try {
        const fd = new FormData();
        fd.append('file', s.file);
        s.result = await api.call('POST', '/sales/import', fd);
        notify('ok', `导入完成：${s.result.imported_rows} 行入账，${s.result.skipped_duplicate_refs} 行重复跳过`);
        await loadBatches();
      } catch (e) { s.err = e.message; } finally { s.busy = false; }
    };
    onMounted(loadBatches);
    return { s, pick, doImport, yuan };
  },
  template: `
  <div class="card">
    <h3>按期导入销量 CSV</h3>
    <div class="muted small">
      表头：<span class="mono">ref,work_code,sale_date,period,region,language,format,qty,unit_price,amount,original_ref</span><br/>
      ref 为渠道行号（全局唯一）；qty 为负表示退货并在 original_ref 填写原销售行号。<br/>
      同一文件重复导入会被整批拒绝（内容指纹），同渠道行号跨文件也不会重复计酬。
    </div>
    <div class="mt12"><input type="file" accept=".csv" @change="pick" /></div>
    <div v-if="s.err" class="flash err mt12">{{s.err}}</div>
    <div class="mt12"><button class="primary" :disabled="s.busy||!s.file" @click="doImport">{{s.busy?'导入并结算中…':'导入并重算结算'}}</button></div>
    <div v-if="s.result" class="mt12">
      <div class="flash ok">批次 #{{s.result.batch_id}}（{{s.result.period}}）：入账 {{s.result.imported_rows}} 行，重复跳过 {{s.result.skipped_duplicate_refs}} 行</div>
      <div v-for="(e,i) in s.result.errors" :key="i" class="flash info" style="margin-bottom:6px">{{e}}</div>
    </div>
  </div>
  <div class="card">
    <h3>已导入批次</h3>
    <table>
      <thead><tr><th>#</th><th>期号</th><th>文件</th><th class="num">行数</th><th>导入时间</th></tr></thead>
      <tbody><tr v-for="b in s.batches" :key="b.id">
        <td>{{b.id}}</td><td class="mono">{{b.period}}</td><td class="small">{{b.file_name}}</td>
        <td class="num">{{b.row_count}}</td><td class="small">{{b.imported_at.slice(0,16).replace('T',' ')}}</td>
      </tr></tbody>
    </table>
  </div>`,
};

// ---------- 异常销量 ----------
const ExceptionsView = {
  setup() {
    const s = reactive({ items: [] });
    onMounted(async () => { s.items = await api.call('GET', '/sales/exceptions'); });
    return { s, yuan, CAT_LABEL, regionName, langName };
  },
  template: `
  <div class="card">
    <h3>异常销量（隔离，不计入正常应付）</h3>
    <div class="muted small mb8">跨区/超授权语种销售、跨期退货、销售时无有效合同（到期后补传）单独列出，财务核对后线下处理。</div>
    <table>
      <thead><tr><th>分类</th><th>作品</th><th>渠道行号</th><th>销售日</th><th>期号</th><th>区域/语种</th><th>类型</th><th class="num">数量</th><th class="num">金额</th><th>原因</th></tr></thead>
      <tbody><tr v-for="r in s.items" :key="r.id">
        <td><span class="badge" :class="r.category">{{CAT_LABEL[r.category]}}</span></td>
        <td class="small">{{r.work_title}}</td>
        <td class="small mono">{{r.ref}}<div v-if="r.original_ref" class="muted">退货自 {{r.original_ref}}</div></td>
        <td class="small mono">{{r.sale_date.slice(0,10)}}</td><td class="mono">{{r.period}}</td>
        <td class="small">{{regionName(r.region)}}/{{langName(r.language)}}</td>
        <td class="small">{{r.format==='print'?'纸书':'电子书'}}</td>
        <td class="num" :class="{'amount-neg':r.qty<0}">{{r.qty}}</td>
        <td class="num" :class="{'amount-neg':r.amount_cents<0}">{{yuan(r.amount_cents)}}</td>
        <td class="small muted" style="max-width:260px">{{r.reason}}</td>
      </tr>
      <tr v-if="!s.items.length"><td colspan="10" class="muted" style="text-align:center;padding:24px">没有异常销量 🎉</td></tr>
      </tbody>
    </table>
  </div>`,
};

// 注册组件
const app = createApp(App);
app.component('login-view', LoginView);
app.component('works-view', WorksView);
app.component('settlements-view', SettlementsView);
app.component('import-view', ImportView);
app.component('exceptions-view', ExceptionsView);

app.mount('#app');
