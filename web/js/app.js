/* Open Dev Cleaner — root application */
(function () {
  'use strict';
  const { createApp, reactive } = Vue;

  const RISKS = [
    { key: 'safe', label: '安全', desc: '纯缓存,工具自动重建' },
    { key: 'caution', label: '谨慎', desc: '需重新下载或重建' },
    { key: 'high', label: '高风险', desc: '需重新安装依赖/镜像' },
    { key: 'dangerous', label: '危险', desc: '数据类,删除后不可恢复' },
  ];

  const app = createApp({
    template: `
      <div>
        <header class="topbar">
          <div class="brand">
            <span class="logo">🧹</span>
            <span>Open Dev Cleaner</span>
            <span class="ver">v{{ version.version }} · {{ version.goos }}</span>
          </div>
          <div class="spacer"></div>
          <a class="gh-link" href="https://github.com/LinuxSuRen/open-dev-cleaner"
            target="_blank" rel="noopener" title="GitHub 仓库 · Star 欢迎Star">
            <svg viewBox="0 0 16 16" width="20" height="20" aria-hidden="true">
              <path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/>
            </svg>
          </a>
          <button class="btn ghost" @click="toggleTheme" :title="'切换主题'">
            {{ theme === 'light' ? '🌙' : '☀️' }}
          </button>
          <button class="btn primary" :disabled="scanning" @click="startScan">
            {{ scanning ? '扫描中…' : (targets.length ? '重新扫描' : '开始扫描') }}
          </button>
        </header>

        <main class="main">
          <div v-if="scanning || progressText" class="scan-progress">
            <span class="spinner" v-if="scanning"></span>
            <span>{{ progressText }}<template v-if="scanning && progressTotal"> ({{ progressDone }}/{{ progressTotal }} 工具)</template></span>
            <span style="margin-left:auto;color:var(--text-dim)">首次扫描大目录可能需要几十秒</span>
          </div>
          <div v-if="fromCache && !scanning && targets.length" class="scan-progress cached">
            <span>⏱</span>
            <span>显示缓存结果(缓存于 {{ cacheTime }}),点击"重新扫描"获取最新数据</span>
          </div>
          <div v-if="scanError" class="scan-progress" style="background:var(--dangerous-weak);color:var(--dangerous)">
            ⚠️ {{ scanError }}
          </div>

          <template v-if="disks.length">
            <div class="disk-card">
              <div class="disk-head">
                <span class="disk-title">💾 磁盘空间</span>
                <span class="disk-total-free">合计剩余 <b>{{ fmt(disks.reduce((a, d) => a + (d.free || 0), 0)) }}</b></span>
              </div>
              <div v-for="d in disks" :key="d.path" class="disk-row">
                <div class="disk-name" :title="diskTitle(d)">
                  <span>{{ diskIcon(d) }}</span>
                  <span class="disk-name-text">{{ diskTitle(d) }}</span>
                </div>
                <div class="disk-bar">
                  <div class="disk-fill" :class="diskBarClass(d)"
                    :style="{ width: (d.total ? (d.used / d.total) * 100 : 0) + '%' }"></div>
                </div>
                <div class="disk-meta">
                  剩余 <b :class="'free-' + diskBarClass(d)">{{ fmt(d.free) }}</b> / 总 {{ fmt(d.total) }}
                </div>
              </div>
            </div>
          </template>

          <template v-if="targets.length">
            <div class="summary-grid">
              <div class="stat-card total">
                <div class="num">{{ fmt(totalSize) }}</div>
                <div class="lbl">可释放空间(共 {{ targets.length }} 组)</div>
              </div>
              <div class="stat-card safe">
                <div class="num">{{ fmt(byRisk.safe) }}</div>
                <div class="lbl">安全 · 纯缓存自动重建</div>
              </div>
              <div class="stat-card caution">
                <div class="num">{{ fmt(byRisk.caution) }}</div>
                <div class="lbl">谨慎 · 需重新下载/重建</div>
              </div>
              <div class="stat-card high">
                <div class="num">{{ fmt(byRisk.high) }}</div>
                <div class="lbl">高风险 · 需重新安装</div>
              </div>
              <div class="stat-card dangerous">
                <div class="num">{{ fmt(byRisk.dangerous) }}</div>
                <div class="lbl">危险 · 不可恢复</div>
              </div>
            </div>
            <div class="risk-bars" :title="'按风险等级占比'">
              <div class="seg-safe" :style="{ width: totalPercent.safe + '%' }"></div>
              <div class="seg-caution" :style="{ width: totalPercent.caution + '%' }"></div>
              <div class="seg-high" :style="{ width: totalPercent.high + '%' }"></div>
              <div class="seg-dangerous" :style="{ width: totalPercent.dangerous + '%' }"></div>
            </div>

            <div class="toolbar">
              <button class="chip" :class="{ active: riskFilter === 'all' }" @click="riskFilter = 'all'">
                全部
              </button>
              <button v-for="r in riskList" :key="r.key" class="chip"
                :class="{ active: riskFilter === r.key }" @click="riskFilter = r.key"
                :title="r.desc">
                <span class="dot" :style="{ background: 'var(--' + r.key + ')' }"></span>
                {{ r.label }} {{ fmt(byRisk[r.key] || 0) }}
              </button>
              <div class="search">
                <span>🔎</span>
                <input v-model="search" placeholder="搜索名称或路径,如 node_modules、gradle…">
              </div>
              <button class="btn small" @click="selectAllVisible">全选可见</button>
              <button class="btn small" @click="clearSelection">取消全选</button>
            </div>

            <div class="layout">
              <aside class="sidenav" v-if="superGroups.length">
                <div class="nav-group" v-for="grp in superGroups" :key="grp.title">
                  <div class="nav-group-title">{{ grp.title }}</div>
                  <a v-for="g in grp.tools" :key="g.tool" class="nav-item"
                    :class="{ active: activeTool === g.tool }" @click.prevent="scrollTo(g.tool)"
                    :title="g.title + ' · ' + g.count + ' 组 · ' + fmt(g.totalSize)">
                    <span class="icon">{{ g.icon }}</span>
                    <span class="nav-name">{{ g.title }}</span>
                    <span class="nav-size">{{ fmt(g.totalSize) }}</span>
                  </a>
                </div>
              </aside>

              <div class="content">
                <odc-tool-section v-for="g in grouped" :key="g.tool"
                  :tool-key="g.tool" :tool-title="g.title" :targets="g.targets" :selection="selection">
                </odc-tool-section>
              </div>
            </div>
          </template>

          <div v-else-if="!scanning" class="empty-state">
            <div class="big">🧹</div>
            <template v-if="summary === null">
              <p style="font-size:16px;font-weight:600">扫描本机,找出可释放的磁盘空间</p>
              <p style="margin-top:0">Go / Node / Java / Python / Rust / Docker / AI 工具 / 工作区构建产物……<br>按危险等级分级展示,默认只勾选安全项</p>
              <button class="btn primary" style="font-size:15px;padding:10px 26px" @click="startScan">🔍 开始扫描</button>
            </template>
            <template v-else>
              <p>没有发现可清理的内容</p>
              <button class="btn primary" @click="startScan">重新扫描</button>
            </template>
          </div>
        </main>

        <footer class="clean-bar" v-if="targets.length">
          <div class="sel-info">
            <div class="big">已选 {{ selectedTargets.length }} 项 · 约 {{ fmt(selectedSize) }}</div>
            <div class="sub">默认勾选"安全"级;谨慎/高风险/危险项请按需手动勾选</div>
          </div>
          <button class="btn" @click="clearSelection" :disabled="cleaning">清空选择</button>
          <button class="btn primary" :disabled="!canClean" @click="openConfirm">
            清理选中项
          </button>
        </footer>

        <div v-if="showConfirm" class="modal-mask" @click.self="closeConfirm">
          <div class="modal">
            <div class="modal-head">🧹 确认清理</div>
            <div class="modal-body">
              <p>即将清理以下 <b>{{ selectedTargets.length }}</b> 项,预计释放 <b>{{ fmt(selectedSize) }}</b>:</p>
              <ul class="confirm-list">
                <li v-for="t in selectedTargets" :key="t.id">
                  <risk-badge :risk="t.risk"></risk-badge>
                  <span>{{ t.title }}</span>
                  <span class="sz">{{ fmt(t.size) }}</span>
                </li>
              </ul>
              <div v-if="selectedHasDangerous" class="danger-zone">
                ⚠️ 包含<b>危险</b>等级目标(如 Docker 数据卷),删除后<b>不可恢复</b>!
                请输入 <b>DELETE</b> 或 <b>删除</b> 以确认:
                <input v-model="dangerText" placeholder="DELETE" autocomplete="off">
              </div>
            </div>
            <div class="modal-foot">
              <button class="btn" @click="closeConfirm" :disabled="cleaning">取消</button>
              <button class="btn" :class="selectedHasDangerous ? 'danger' : 'primary'"
                :disabled="cleaning || (selectedHasDangerous && !dangerReady)"
                @click="doClean">
                {{ selectedHasDangerous ? '确认删除' : '开始清理' }}
              </button>
            </div>
          </div>
        </div>

        <div v-if="showLog" class="modal-mask">
          <div class="modal">
            <div class="modal-head">
              <span v-if="cleaning" class="spinner"></span>
              {{ cleaning ? '正在清理…' : (cleanDone ? '清理完成' : '清理日志') }}
            </div>
            <div class="modal-body">
              <div class="log-view">
                <div v-for="(l, i) in logs" :key="i" class="log-line" :class="l.cls">{{ l.msg }}</div>
              </div>
              <p v-if="cleanDone && cleanSummaryText" style="margin-bottom:0">
                ✅ {{ cleanSummaryText }}
              </p>
            </div>
            <div class="modal-foot">
              <button v-if="cleanDone" class="btn" @click="showLog = false">关闭</button>
              <button v-if="cleanDone" class="btn primary" @click="showLog = false; startScan()">重新扫描</button>
            </div>
          </div>
        </div>
      </div>
    `,

    data() {
      return {
        version: { version: '…', goos: '' },
        disks: [],
        scanning: false,
        scanError: '',
        progressText: '',
        progressDone: 0,
        progressTotal: 0,
        targets: [],
        summary: null,
        selection: reactive({}),
        riskFilter: 'all',
        search: '',
        showConfirm: false,
        dangerText: '',
        showLog: false,
        cleaning: false,
        logs: [],
        cleanDone: false,
        cleanSummaryText: '',
        theme: localStorage.getItem('odc-theme') || 'light',
        activeTool: '',
        fromCache: false,
        cacheTime: '',
      };
    },

    computed: {
      fmt() { return ODC.fmtSize; },
      riskList() { return RISKS; },
      byRisk() {
        const out = { safe: 0, caution: 0, high: 0, dangerous: 0 };
        for (const t of this.targets) out[t.risk] = (out[t.risk] || 0) + (t.size || 0);
        return out;
      },
      totalSize() { return this.targets.reduce((a, t) => a + (t.size || 0), 0); },
      totalPercent() {
        if (!this.totalSize) return { safe: 0, caution: 0, high: 0, dangerous: 0 };
        return {
          safe: (this.byRisk.safe / this.totalSize) * 100,
          caution: (this.byRisk.caution / this.totalSize) * 100,
          high: (this.byRisk.high / this.totalSize) * 100,
          dangerous: (this.byRisk.dangerous / this.totalSize) * 100,
        };
      },
      grouped() {
        const groups = [];
        const byTool = new Map();
        for (const t of this.targets) {
          if (!this.matchFilter(t)) continue;
          let g = byTool.get(t.tool);
          if (!g) {
            g = { tool: t.tool, title: t.toolTitle, targets: [] };
            byTool.set(t.tool, g);
            groups.push(g);
          }
          g.targets.push(t);
        }
        return groups;
      },

      // Super categories for the left navigation.
      superGroups() {
        const order = ['语言与运行时', '容器与模型', '研发/运维', 'AI 编程工具', '工作区', 'IDE 与编辑器', '工具链版本', '应用缓存', '系统', '其他'];
        const map = {
          go: '语言与运行时', node: '语言与运行时', java: '语言与运行时',
          python: '语言与运行时', rust: '语言与运行时', dotnet: '语言与运行时',
          docker: '容器与模型', ollama: '容器与模型', kube: '容器与模型',
          infra: '研发/运维',
          ai: 'AI 编程工具',
          workspace: '工作区', git: '工作区',
          editor: 'IDE 与编辑器',
          versions: '工具链版本',
          apps: '应用缓存', browser: '应用缓存',
          os: '系统',
        };
        const icons = {
          docker: '🐳', go: '🐹', node: '🟢', java: '☕', python: '🐍', rust: '🦀',
          dotnet: '🟣', workspace: '📁', ollama: '🦙', ai: '🤖', editor: '🧑‍💻',
          versions: '🔧', apps: '🧩', browser: '🌐', os: '⚙️', kube: '☸️', infra: '🛠️', git: '🌿',
        };
        const buckets = new Map();
        for (const g of this.grouped) {
          const cat = map[g.tool] || '其他';
          if (!buckets.has(cat)) buckets.set(cat, []);
          buckets.get(cat).push({
            tool: g.tool, title: g.title,
            totalSize: g.targets.reduce((a, t) => a + (t.size || 0), 0),
            count: g.targets.length,
            icon: icons[g.tool] || '📦',
          });
        }
        return order
          .filter((c) => buckets.has(c))
          .map((c) => ({ title: c, tools: buckets.get(c) }));
      },
      flatSelection() {
        const ids = [];
        const walk = (t) => {
          if (this.selection[t.id]) ids.push(t.id);
          for (const c of t.items || []) walk(c);
        };
        for (const t of this.targets) walk(t);
        return ids;
      },
      selectedTargets() {
        // top-level and child targets currently selected, excluding children
        // whose ancestor group is already selected
        const out = [];
        const walk = (t, ancestorSelected) => {
          const sel = !ancestorSelected && this.selection[t.id];
          if (sel) out.push(t);
          for (const c of t.items || []) walk(c, ancestorSelected || sel);
        };
        for (const t of this.targets) walk(t, false);
        return out;
      },
      selectedSize() { return this.selectedTargets.reduce((a, t) => a + (t.size || 0), 0); },
      selectedHasDangerous() { return this.selectedTargets.some((t) => t.risk === 'dangerous'); },
      dangerReady() { return this.dangerText.trim().toUpperCase() === 'DELETE' || this.dangerText.trim() === '删除'; },
      canClean() { return !this.cleaning && this.selectedTargets.length > 0; },
      scanFinished() { return !this.scanning && this.targets.length >= 0 && this.summary !== null; },
    },

    methods: {
      riskLabel: ODC.riskLabel,

      matchFilter(t) {
        const self = this;
        const matchOne = (x) => {
          if (self.riskFilter !== 'all' && x.risk !== self.riskFilter) return false;
          if (self.search) {
            const q = self.search.toLowerCase();
            const hay = (x.title + ' ' + (x.path || '') + ' ' + (x.description || '')).toLowerCase();
            if (!hay.includes(q)) return false;
          }
          return true;
        };
        const matchDeep = (x) => matchOne(x) || (x.items || []).some(matchDeep);
        return matchDeep(t);
      },

      applyTheme() {
        document.documentElement.setAttribute('data-theme', this.theme);
        localStorage.setItem('odc-theme', this.theme);
      },
      toggleTheme() {
        this.theme = this.theme === 'light' ? 'dark' : 'light';
        this.applyTheme();
      },

      scrollTo(tool) {
        this.activeTool = tool;
        const el = document.getElementById('sec-' + tool);
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
      },

      // --- 扫描结果缓存 (sessionStorage) ---
      // 避免高频点击扫描/刷新页面造成重复全盘计算;清理后失效。
      cacheKey() {
        return 'odc-scan-' + (this.version && this.version.version ? this.version.version : 'unknown');
      },
      persistScanCache() {
        try {
          const payload = {
            version: this.version.version,
            time: Date.now(),
            targets: this.targets,
            summary: this.summary,
            selection: Object.assign({}, this.selection),
          };
          sessionStorage.setItem(this.cacheKey(), JSON.stringify(payload));
        } catch (e) { /* quota exceeded etc. — 缓存失败不影响功能 */ }
      },
      restoreScanCache() {
        try {
          const raw = sessionStorage.getItem(this.cacheKey());
          if (!raw) return;
          const payload = JSON.parse(raw);
          if (!payload || !payload.targets || !payload.targets.length) return;
          if (payload.version !== this.version.version) return; // 版本变化,结构可能不兼容
          this.targets = payload.targets;
          this.summary = payload.summary || null;
          if (payload.selection) {
            for (const k of Object.keys(this.selection)) delete this.selection[k];
            Object.assign(this.selection, payload.selection);
          }
          this.cacheTime = new Date(payload.time).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
          this.fromCache = true;
        } catch (e) { /* 损坏的缓存直接忽略 */ }
      },
      invalidateScanCache() {
        try { sessionStorage.removeItem(this.cacheKey()); } catch (e) { /* ignore */ }
        this.fromCache = false;
      },

      setupSpy() {
        if (typeof IntersectionObserver === 'undefined') return;
        if (this._spy) this._spy.disconnect();
        const sections = document.querySelectorAll('[data-section]');
        if (!sections.length) return;
        this._spy = new IntersectionObserver((entries) => {
          for (const e of entries) {
            if (e.isIntersecting) {
              this.activeTool = e.target.getAttribute('data-section');
            }
          }
        }, { rootMargin: '-72px 0px -60% 0px', threshold: 0 });
        sections.forEach((s) => this._spy.observe(s));
      },

      async loadVersion() {
        try {
          const res = await fetch('/api/version');
          this.version = await res.json();
        } catch (e) { /* ignore */ }
      },

      async loadDisks() {
        try {
          const res = await fetch('/api/disks');
          const data = await res.json();
          this.disks = data.disks || [];
        } catch (e) { /* ignore */ }
      },

      diskBarClass(v) {
        const pct = v.total ? (v.used / v.total) * 100 : 0;
        if (pct >= 90) return 'crit';
        if (pct >= 70) return 'warn';
        return 'ok';
      },
      diskIcon(v) {
        return { fixed: '💽', removable: '🔌', remote: '🌐', ramdisk: '🧠', readonly: '🔒', volume: '💾' }[v.kind] || '💾';
      },
      diskTitle(v) {
        let name = v.label ? v.label + ' (' + v.path + ')' : v.path;
        if (v.kind === 'removable') name += ' · 可移动';
        if (v.kind === 'remote') name += ' · 网络驱动器';
        if (v.fsType) name += ' · ' + v.fsType;
        return name;
      },

      startScan() {
        if (this.scanning) return;
        this.scanning = true;
        this.scanError = '';
        this.summary = null;
        this.targets = [];
        this.progressDone = 0;
        this.progressTotal = 0;
        this.progressText = '正在启动扫描…';
        for (const k of Object.keys(this.selection)) delete this.selection[k];

        const es = new EventSource('/api/scan');
        es.onmessage = (ev) => {
          let data;
          try { data = JSON.parse(ev.data); } catch (e) { return; }
          if (data.type === 'progress') {
            if (data.message === 'scanning') {
              this.progressTotal += 1;
              this.progressText = '正在扫描: ' + data.tool;
            } else if (data.message === 'done') {
              this.progressDone += 1;
              // 计数统一由模板渲染 "(done/total 工具)",这里不再重复拼数字
            }
          } else if (data.type === 'target' && data.target) {
            this.targets.push(data.target);
            // default-select safe & available targets on arrival
            if (data.target.risk === 'safe' && data.target.available) {
              this.selection[data.target.id] = true;
            }
          } else if (data.type === 'done') {
            this.summary = data.summary;
            this.scanning = false;
            this.progressText = '';
            this.fromCache = false;
            this.persistScanCache();
            es.close();
          }
        };
        es.onerror = () => {
          if (this.scanning) {
            this.scanning = false;
            this.scanError = '扫描连接中断,请重试';
          }
          es.close();
        };
      },

      selectAllVisible() {
        const walk = (t) => {
          if (t.available && this.matchFilter(t)) this.selection[t.id] = true;
          for (const c of t.items || []) walk(c);
        };
        for (const t of this.targets) walk(t);
      },
      clearSelection() {
        for (const k of Object.keys(this.selection)) delete this.selection[k];
      },

      openConfirm() { this.showConfirm = true; this.dangerText = ''; },
      closeConfirm() { if (!this.cleaning) this.showConfirm = false; },

      async doClean() {
        this.showConfirm = false;
        this.showLog = true;
        this.cleaning = true;
        this.cleanDone = false;
        this.logs = [];
        this.cleanSummaryText = '';
        const log = (msg, cls) => {
          this.logs.push({ msg, cls: cls || '' });
        };
        log('开始清理 ' + this.selectedTargets.length + ' 项…');

        try {
          const res = await fetch('/api/clean', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              ids: this.flatSelection,
              dangerConfirm: this.dangerText.trim().toUpperCase() === 'DELETE' || this.dangerText.trim() === '删除',
            }),
          });
          if (!res.ok || !res.body) {
            const text = await res.text();
            let msg = text;
            try { msg = JSON.parse(text).error || text; } catch (e) { /* raw */ }
            log('请求失败: ' + msg, 'err');
            this.cleaning = false;
            this.cleanDone = true;
            return;
          }

          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buf = '';
          for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            buf += decoder.decode(value, { stream: true });
            for (;;) {
              const idx = buf.indexOf('\n\n');
              if (idx < 0) break;
              const chunk = buf.slice(0, idx);
              buf = buf.slice(idx + 2);
              if (!chunk.startsWith('data: ')) continue;
              let data;
              try { data = JSON.parse(chunk.slice(6)); } catch (e) { continue; }
              if (data.type === 'log') {
                const cls = data.message.startsWith('[完成]') || data.message.startsWith('  ')
                  ? 'ok' : (data.message.startsWith('[失败]') || data.message.startsWith('[拒绝]') ? 'err' : '');
                log(data.message, cls);
              } else if (data.type === 'done') {
                log(data.message, 'ok');
                this.cleanSummaryText = data.message;
              }
            }
          }
        } catch (e) {
          log('网络错误: ' + e, 'err');
        }
        this.cleaning = false;
        this.cleanDone = true;
        this.loadDisks(); // refresh free space after cleaning
        this.invalidateScanCache(); // 清理后结果已变化,缓存失效
      },
    },

    watch: {
      grouped() {
        this.$nextTick(() => this.setupSpy());
      },
    },

    mounted() {
      this.applyTheme();
      this.loadVersion().then(() => this.restoreScanCache());
      this.loadDisks(); // disk overview is cheap; scanning stays manual
    },
  });

  app.component('risk-badge', window.ODCRiskBadge);
  app.component('odc-target-row', window.OdcTargetRow);
  app.component('odc-tool-section', window.OdcToolSection);

  app.mount('#app');
})();
