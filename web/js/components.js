/* global shared helpers + components (registered on window.Vue) */
(function () {
  'use strict';

  const RISK_LABELS = { safe: '安全', caution: '谨慎', high: '高风险', dangerous: '危险' };

  window.ODC = {
    RISK_LABELS,
    riskLabel(r) { return RISK_LABELS[r] || r; },
    fmtSize(n) {
      if (n == null || isNaN(n)) return '-';
      if (n < 1024) return n + ' B';
      const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
      let v = n, i = -1;
      do { v /= 1024; i++; } while (v >= 1024 && i < units.length - 1);
      return (v >= 100 ? v.toFixed(0) : v.toFixed(1)) + ' ' + units[i];
    },
    fmtNum(n) { return (n || 0).toLocaleString('zh-CN'); },
  };

  const { createApp } = Vue;

  /* ---------- RiskBadge ---------- */
  window.ODCRiskBadge = {
    name: 'RiskBadge',
    props: { risk: String, label: String },
    computed: {
      text() { return this.label || ODC.riskLabel(this.risk); },
    },
    template: '<span class="badge" :class="risk">{{ text }}</span>',
  };

  /* ---------- TargetRow (recursive) ---------- */
  window.OdcTargetRow = {
    name: 'TargetRow',
    props: {
      target: { type: Object, required: true },
      selection: { type: Object, required: true },
      parentSelected: { type: Boolean, default: false },
    },
    data() { return { expanded: false }; },
    computed: {
      checked: {
        get() { return !!this.selection[this.target.id]; },
        set(v) { if (v) this.selection[this.target.id] = true; else delete this.selection[this.target.id]; },
      },
      sumSize() { return this.target.size; },
      childCount() { return (this.target.items || []).length; },
      icon() {
        const icons = {
          docker: '🐳', go: '🐹', node: '🟢', java: '☕', python: '🐍',
          rust: '🦀', workspace: '📁', ollama: '🦙', editor: '🧑‍💻',
          apps: '🧩', browser: '🌐', os: '⚙️', ai: '🤖', versions: '🔧',
          dotnet: '🟣', kube: '☸️',
        };
        return icons[this.target.tool] || '📦';
      },
    },
    methods: {
      fmt: ODC.fmtSize,
      fmtNum: ODC.fmtNum,
      toggleExpand() { this.expanded = !this.expanded; },
    },
    template: `
      <div>
        <div class="target-row" :class="{ disabled: !target.available }">
          <input v-if="target.available" type="checkbox" v-model="checked">
          <input v-else type="checkbox" disabled>
          <div class="t-body">
            <div class="t-title-row">
              <span class="t-title">{{ target.title }}</span>
              <risk-badge :risk="target.risk"></risk-badge>
              <span class="badge cat">{{ target.category }}</span>
              <button v-if="childCount" class="expand-btn" @click="toggleExpand">
                {{ expanded ? '▾ 收起' : '▸ 展开' }} {{ childCount }} 项
              </button>
            </div>
            <div class="t-desc">{{ target.description }}</div>
            <div v-if="target.path" class="t-path" :title="target.path">{{ target.path }}</div>
            <div v-if="target.note" class="row-note" :class="{ warn: !target.available }">ⓘ {{ target.note }}</div>
          </div>
          <div class="t-size">
            {{ fmt(sumSize) }}
            <span v-if="target.count" class="t-files">{{ fmtNum(target.count) }} 个文件</span>
          </div>
        </div>
        <div v-if="expanded && childCount" class="children">
          <odc-target-row
            v-for="c in target.items" :key="c.id"
            :target="c" :selection="selection"
            :parent-selected="parentSelected || checked">
          </odc-target-row>
        </div>
      </div>
    `,
    components: { 'risk-badge': window.ODCRiskBadge },
  };
  // allow self reference
  window.OdcTargetRow.components['odc-target-row'] = window.OdcTargetRow;

  /* ---------- ToolSection ---------- */
  window.OdcToolSection = {
    name: 'ToolSection',
    props: {
      toolKey: String, toolTitle: String, targets: Array, selection: Object,
    },
    computed: {
      totalSize() { return this.targets.reduce((a, t) => a + (t.size || 0), 0); },
      icon() {
        const icons = {
          docker: '🐳', go: '🐹', node: '🟢', java: '☕', python: '🐍', rust: '🦀',
          workspace: '📁', ollama: '🦙', editor: '🧑‍💻', apps: '🧩', browser: '🌐', os: '⚙️',
          ai: '🤖', versions: '🔧', dotnet: '🟣', kube: '☸️',
        };
        return icons[this.toolKey] || '📦';
      },
    },
    methods: {
      fmt: ODC.fmtSize,
      allChecked() {
        const avail = this.targets.filter((t) => t.available);
        return avail.length > 0 && avail.every((t) => this.selection[t.id]);
      },
      toggleAll() {
        const on = !this.allChecked();
        for (const t of this.targets) {
          if (!t.available) continue;
          if (on) this.selection[t.id] = true;
          else delete this.selection[t.id];
        }
      },
    },
    template: `
      <div class="tool-section" :id="'sec-' + toolKey" :data-section="toolKey">
        <div class="tool-head">
          <input type="checkbox" :checked="allChecked()" @change="toggleAll">
          <span class="icon">{{ icon }}</span>
          <span class="name">{{ toolTitle }}</span>
          <span class="meta">{{ targets.length }} 项</span>
          <span class="size">{{ fmt(totalSize) }}</span>
        </div>
        <odc-target-row v-for="t in targets" :key="t.id" :target="t" :selection="selection">
        </odc-target-row>
      </div>
    `,
    components: { 'odc-target-row': window.OdcTargetRow },
  };
})();
