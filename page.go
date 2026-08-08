package agw

import (
	"html/template"
	"net/http"
	"strings"
)

type configView struct {
	Index            int
	Name             string
	URL              string
	AuthType         string
	AuthValue        string
	AppSelectorsText string
	HasAuth          bool
}

type pageView struct {
	Debug        bool
	AppSelectors []AppSelector
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AGW Control</title>
  <script>try { document.documentElement.dataset.theme = localStorage.getItem('agw-theme') || 'dark'; } catch (_) { document.documentElement.dataset.theme = 'dark'; }</script>
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>
  <script src="https://unpkg.com/lucide@0.468.0"></script>
  <style>
    :root { color-scheme: dark; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    :root[data-theme="light"] { color-scheme: light; }
    * { box-sizing: border-box; }
    .sr-only { position: absolute; inset: 0; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
    body { margin: 0; color: #1b2927; background: #eef2f0; }
    button, input, select { font: inherit; }
    .shell { width: min(1220px, calc(100% - 40px)); margin: 28px auto 40px; }
    .appbar { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 18px 20px; color: #f4f9f7; background: #12312d; border: 1px solid #20473f; border-radius: 8px 8px 0 0; }
    .identity { display: flex; align-items: center; gap: 12px; min-width: 0; }
    .brand-mark { display: grid; width: 34px; height: 34px; place-items: center; flex: 0 0 34px; color: #17302b; background: #cbe86b; border-radius: 6px; font-size: 12px; font-weight: 800; letter-spacing: 0; }
    .eyebrow { margin: 0 0 2px; color: #9ebcb5; font-size: 11px; font-weight: 700; letter-spacing: 0; text-transform: uppercase; }
    h1 { margin: 0; font-size: 20px; font-weight: 700; letter-spacing: 0; }
    .appbar-actions { display: flex; align-items: center; gap: 8px; }
    .debug-toggle { display: inline-flex; align-items: center; gap: 8px; min-height: 34px; padding: 0 8px; color: #d7e6e1; cursor: pointer; font-size: 13px; white-space: nowrap; }
    .debug-toggle input { position: absolute; opacity: 0; pointer-events: none; }
    .switch { display: inline-flex; width: 30px; height: 18px; align-items: center; padding: 2px; background: #41635d; border-radius: 999px; transition: background .16s ease; }
    .switch::after { width: 14px; height: 14px; background: #f7fbf9; border-radius: 50%; content: ""; transition: transform .16s ease; }
    .debug-toggle input:checked + .switch { background: #8eb637; }
    .debug-toggle input:checked + .switch::after { transform: translateX(12px); }
    .icon-button { display: inline-grid; width: 34px; height: 34px; place-items: center; padding: 0; color: #31524b; background: #fff; border: 1px solid #c8d4d0; border-radius: 5px; cursor: pointer; transition: background .15s ease, border-color .15s ease, color .15s ease; }
    .icon-button svg { width: 16px; height: 16px; }
    .icon-button:hover { color: #176d59; background: #f2f8f5; border-color: #8fb7aa; }
    .icon-button:focus-visible, .field-input:focus, .auth-select:focus { outline: 2px solid #62a58d; outline-offset: 1px; }
    .icon-button.save { color: #17372f; background: #cbe86b; border-color: #cbe86b; }
    .icon-button.save:hover { background: #b9d755; border-color: #b9d755; }
    .icon-button.danger { color: #9e3e3e; }
    .icon-button.danger:hover { color: #862f2f; background: #fff1f0; border-color: #e3aaa5; }
    .workspace { background: #fff; border: 1px solid #d7e0dc; border-top: 0; border-radius: 0 0 8px 8px; box-shadow: 0 12px 28px rgba(24, 44, 39, .06); }
    .selector-workspace { margin-top: 20px; border-top: 1px solid #d7e0dc; border-radius: 8px; }
    .workspace-top { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 20px 14px; border-bottom: 1px solid #e5ece9; }
    .section-title { margin: 0; color: #213633; font-size: 15px; font-weight: 700; }
    .section-note { margin: 3px 0 0; color: #71827e; font-size: 12px; }
    .section-actions { display: flex; align-items: center; gap: 10px; }
    .summary { display: flex; align-items: center; gap: 8px; color: #53716a; font-size: 12px; white-space: nowrap; }
    .summary-dot { width: 7px; height: 7px; background: #7fae33; border-radius: 50%; }
    #status { min-width: 72px; color: #637570; font-size: 12px; text-align: right; }
    #status.error { color: #a83e3e; }
    #status.success { color: #267d63; }
    .table-scroll { overflow-x: auto; padding: 0 10px 10px; background: #f7faf8; }
    table { width: 100%; min-width: 980px; border-collapse: separate; border-spacing: 0 8px; table-layout: fixed; }
    th, td { padding: 12px 14px; text-align: left; vertical-align: middle; }
    th { padding-block: 5px 7px; color: #778984; background: transparent; font-size: 11px; font-weight: 700; letter-spacing: 0; text-transform: uppercase; }
    th.priority, td.priority { width: 76px; text-align: center; white-space: nowrap; }
    th.name { width: 14%; }
    th.endpoint { width: 27%; }
    th.authentication { width: 24%; }
    th.app-selectors { width: 18%; }
    th.row-actions, td.row-actions { width: 96px; text-align: center; white-space: nowrap; }
    td.row-actions .icon-button + .icon-button { margin-left: 6px; }
    tr[data-row] td { background: #fff; border-top: 1px solid #dce5e1; border-bottom: 1px solid #dce5e1; }
    tr[data-row] td:first-child { border-left: 1px solid #dce5e1; border-radius: 6px 0 0 6px; }
    tr[data-row] td:last-child { border-right: 1px solid #dce5e1; border-radius: 0 6px 6px 0; }
    tr[data-row]:hover td { background: #fbfdfc; border-color: #bdcdc6; }
    tr[data-row], .selector-row { transition: transform .18s ease, opacity .16s ease, border-color .16s ease, background .16s ease; }
    tr.dragging, .selector-row.dragging { opacity: .34; filter: saturate(.65); }
    .drag-handle { display: inline-grid; width: 28px; height: 32px; place-items: center; color: #91a29d; cursor: grab; user-select: none; }
    .drag-handle svg { width: 16px; height: 16px; }
    .field-input, .auth-select { width: 100%; height: 34px; min-width: 0; padding: 6px 8px; color: #203330; background: transparent; border: 1px solid transparent; border-radius: 4px; }
    .field-input { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 13px; }
    .field-input:hover, .auth-select:hover, .field-input:focus, .auth-select:focus { background: #fff; border-color: #b8c9c3; }
    .auth { display: grid; grid-template-columns: 112px minmax(0, 1fr) 34px; gap: 7px; align-items: center; }
    .auth-select { color: #30564d; font-size: 13px; font-weight: 600; }
    .auth-value { min-width: 0; }
    .auth-value .field-input { width: 100%; }
    .multi-select { position: relative; min-width: 0; }
    .ms-trigger { display: flex; min-height: 34px; align-items: center; justify-content: space-between; gap: 6px; padding: 4px 8px; color: #203330; background: transparent; border: 1px solid transparent; border-radius: 4px; cursor: pointer; }
    .ms-trigger:hover { background: #fff; border-color: #b8c9c3; }
    .ms-trigger:focus-visible { background: #fff; border-color: #b8c9c3; outline: 2px solid #62a58d; outline-offset: 1px; }
    .ms-chips { display: flex; flex-wrap: wrap; gap: 4px; min-width: 0; }
    .ms-chip { display: inline-flex; max-width: 100%; align-items: center; gap: 3px; padding: 2px 4px 2px 7px; color: #1c4a3d; background: #e5f1ec; border: 1px solid #bcd8cd; border-radius: 999px; font: 11px/1.5 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .ms-chip.is-stale { color: #8a4a1d; background: #fdf0df; border-color: #e5c394; }
    .ms-chip button { display: inline-grid; width: 15px; height: 15px; place-items: center; padding: 0; color: inherit; background: transparent; border: 0; border-radius: 50%; cursor: pointer; }
    .ms-chip button:hover { background: rgba(0, 0, 0, .1); }
    .ms-chip svg { width: 11px; height: 11px; }
    .ms-placeholder { color: #97a7a2; font-size: 12px; white-space: nowrap; }
    .ms-chevron { flex: 0 0 14px; width: 14px; height: 14px; color: #8b9a96; }
    .ms-menu { position: fixed; z-index: 60; display: grid; gap: 2px; width: max-content; min-width: 240px; max-height: 260px; padding: 6px; overflow: auto; color: #203330; background: #fff; border: 1px solid #c8d4d0; border-radius: 6px; box-shadow: 0 14px 30px rgba(24, 44, 39, .16); }
    .ms-menu[hidden] { display: none; }
    .ms-option { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 4px; cursor: pointer; font-size: 13px; }
    .ms-option:hover { background: #f0f6f3; }
    .ms-option input { width: 15px; height: 15px; margin: 0; accent-color: #4d8f79; }
    .ms-option.is-stale { color: #9a6b1b; }
    .ms-empty { padding: 12px; color: #7b8b87; font-size: 12px; text-align: center; }
    .selector-table-head { display: grid; grid-template-columns: 180px minmax(0, 1fr) 120px; gap: 12px; padding: 9px 20px 7px; color: #778984; background: #f7faf8; font-size: 11px; font-weight: 700; text-transform: uppercase; }
    .selector-list { display: grid; gap: 8px; padding: 10px; background: #f7faf8; }
    .selector-row { display: grid; grid-template-columns: 180px minmax(0, 1fr) 120px; gap: 12px; align-items: start; padding: 12px; background: #fff; border: 1px solid #dce5e1; border-radius: 6px; }
    .selector-row:hover { border-color: #bdcdc6; background: #fbfdfc; }
    .selector-name-cell { display: grid; grid-template-columns: 28px minmax(0, 1fr); gap: 5px; align-items: center; }
    .selector-matches { display: grid; gap: 6px; min-width: 0; min-height: 48px; padding: 6px; background: #f7faf8; border: 1px solid #d5e0db; border-radius: 5px; }
    .rule { display: grid; grid-template-columns: 58px minmax(0, 1fr) 40px 34px; gap: 6px; align-items: center; padding: 6px; background: #fff; border: 1px solid #d5e0db; border-radius: 5px; }
    .rule.is-disabled { opacity: .45; }
    .rule-kind { font-size: 10px; font-weight: 800; text-transform: uppercase; letter-spacing: .05em; color: #3f8b73; white-space: nowrap; }
    .rule[data-rule-type="path"] .rule-kind { color: #9a6b1b; }
    .rule[data-rule-type="body"] .rule-kind { color: #5a6db8; }
    .rule[data-rule-type="query"] .rule-kind { color: #7b5aa6; }
    .rule[data-rule-type="rewrite"] .rule-kind { color: #a44a6e; }
    .rule-controls { display: grid; gap: 6px; min-width: 0; align-items: center; }
    .rule-controls-header, .rule-controls-query, .rule-controls-body { grid-template-columns: minmax(0, 1fr) 110px minmax(0, 1fr); }
    .rule-controls-path { grid-template-columns: 110px minmax(0, 1fr); }
    .rule-controls-rewrite { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
    .rule-switch { position: relative; display: inline-block; width: 38px; height: 22px; }
    .rule-switch input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
    .rule-switch span { position: absolute; inset: 0; background: #b9c7c2; border-radius: 999px; transition: background .15s ease; }
    .rule-switch span::after { content: ""; position: absolute; top: 3px; left: 3px; width: 16px; height: 16px; background: #fff; border-radius: 50%; transition: transform .15s ease; }
    .rule-switch input:checked + span { background: #4d8f79; }
    .rule-switch input:checked + span::after { transform: translateX(16px); }
    .rule-switch input:focus-visible + span { outline: 2px solid #62a58d; outline-offset: 1px; }
    .rule-add { position: relative; display: flex; justify-content: flex-end; }
    .rule-menu { position: absolute; top: calc(100% + 4px); right: 0; z-index: 20; display: grid; gap: 2px; padding: 4px; min-width: 120px; background: #fff; border: 1px solid #d5e0db; border-radius: 6px; box-shadow: 0 10px 24px rgba(20, 60, 48, .16); }
    .rule-menu[hidden] { display: none; }
    .rule-menu button { display: block; width: 100%; padding: 7px 10px; color: #203330; background: transparent; border: 0; border-radius: 4px; cursor: pointer; text-align: left; font-size: 12px; font-weight: 700; }
    .rule-menu button:hover { background: #edf5f1; color: #27634f; }
    .match-value-field { position: relative; min-width: 0; }
    .match-value-field .field-input { padding-right: 66px; }
    .match-value-actions { position: absolute; top: 3px; right: 3px; display: flex; align-items: center; }
    .match-value-actions .icon-button { width: 28px; height: 28px; border: 0; background: transparent; }
    .match-value-actions .rule-clear { color: #9e3e3e; }
    .match-value-actions .rule-clear:hover { color: #862f2f; background: #fff1f0; }
    .match-case-toggle.is-active { color: #17372f; background: #cbe86b; border-color: #cbe86b; }
    .selector-actions { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
    .text-button { display: inline-flex; height: 34px; align-items: center; gap: 5px; padding: 0 4px; color: #327662; background: transparent; border: 0; cursor: pointer; font-size: 11px; font-weight: 700; }
    .text-button svg { width: 14px; height: 14px; }
    .selector-no-rules { display: flex; min-height: 34px; align-items: center; justify-content: space-between; gap: 10px; padding: 0 2px 0 6px; color: #7b8b87; font-size: 12px; }
    .selector-no-rules[hidden] { display: none; }
    .selector-empty { grid-column: 1 / -1; padding: 22px; color: #7b8b87; background: #fff; border: 1px dashed #c9d6d1; border-radius: 6px; font-size: 12px; text-align: center; }
    .drop-indicator td { height: 42px; padding: 0; background: transparent; border: 0; }
    .drop-indicator span { display: grid; height: 30px; place-items: center; color: #3f8b73; background: rgba(92, 165, 137, .08); border: 1px dashed #70a996; border-radius: 5px; font-size: 11px; font-weight: 700; }
    .selector-list > .drop-indicator { display: grid; height: 38px; place-items: center; color: #3f8b73; background: rgba(92, 165, 137, .08); border: 1px dashed #70a996; border-radius: 5px; font-size: 11px; font-weight: 700; }
    .drag-ghost { position: fixed; top: -200px; left: -200px; z-index: 10; padding: 8px 12px; color: #e9f3ef; background: #1c332d; border: 1px solid #6aa18e; border-radius: 5px; box-shadow: 0 10px 22px rgba(0, 0, 0, .22); font-size: 12px; font-weight: 700; pointer-events: none; }
    .reorder-settled { animation: reorder-settle .54s ease; }
    tr.reorder-settled td { animation: reorder-settle-cell .54s ease; }
    @keyframes reorder-settle { 0% { border-color: #8fcf6e; box-shadow: 0 0 0 0 rgba(143, 207, 110, .38); } 100% { border-color: inherit; box-shadow: 0 0 0 10px rgba(143, 207, 110, 0); } }
    @keyframes reorder-settle-cell { 0% { background: #233b31; border-color: #8fcf6e; } 100% { background: inherit; border-color: inherit; } }
    .logs { margin-top: 20px; overflow: hidden; background: #13211f; border: 1px solid #29433e; border-radius: 8px; }
    .logs-top { display: flex; align-items: center; justify-content: space-between; padding: 12px 14px; color: #dce9e5; border-bottom: 1px solid #29433e; }
    .logs-title { display: flex; align-items: center; gap: 8px; margin: 0; font-size: 13px; font-weight: 700; }
    .live-dot { width: 7px; height: 7px; background: #b9e55a; border-radius: 50%; box-shadow: 0 0 0 3px rgba(185, 229, 90, .12); }
    .logs-meta { color: #9db5ae; font: 11px ui-monospace, SFMono-Regular, Menlo, monospace; }
    #log-stream { height: 250px; overflow: auto; padding: 14px; margin: 0; color: #c6d8d2; background: #13211f; white-space: pre-wrap; word-break: break-word; font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    #log-stream::selection { color: #13211f; background: #cbe86b; }
    .telemetry { margin-top: 20px; overflow: hidden; background: #fff; border: 1px solid #d7e0dc; border-radius: 8px; box-shadow: 0 10px 24px rgba(24, 44, 39, .04); }
    .telemetry-tabbar { display: flex; align-items: end; gap: 3px; padding: 10px 12px 0; background: #e9efec; border-bottom: 1px solid #d7e0dc; }
    .telemetry-tab { display: inline-flex; min-height: 35px; align-items: center; gap: 7px; padding: 0 12px; margin-bottom: -1px; color: #60736d; background: transparent; border: 1px solid transparent; border-bottom: 0; border-radius: 6px 6px 0 0; cursor: pointer; font-size: 12px; font-weight: 700; white-space: nowrap; }
    .telemetry-tab:hover { color: #2e5b51; background: rgba(255, 255, 255, .46); }
    .telemetry-tab[aria-selected="true"] { color: #173d34; background: #fff; border-color: #d7e0dc; box-shadow: 0 -1px 2px rgba(21, 47, 40, .06); }
    .telemetry-tab:focus-visible { outline: 2px solid #62a58d; outline-offset: 1px; }
    .telemetry-panel[hidden] { display: none; }
    .tab-connection { color: #7c918a; font: 10px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .session-journal { margin-top: 20px; overflow: hidden; background: #fff; border: 1px solid #d7e0dc; border-radius: 8px; box-shadow: 0 10px 24px rgba(24, 44, 39, .04); }
    .journal-top { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 16px 18px; border-bottom: 1px solid #e5ece9; }
    .journal-title { margin: 0; color: #213633; font-size: 15px; font-weight: 700; }
    .journal-note { margin: 3px 0 0; color: #71827e; font-size: 12px; }
    .session-table-head { display: grid; grid-template-columns: 8px minmax(200px, 1.4fr) minmax(84px, .7fr) minmax(104px, .8fr) minmax(84px, .8fr) 64px 48px 86px 86px 18px; align-items: center; gap: 12px; padding: 8px 24px; color: #778984; background: #eef4f1; border-bottom: 1px solid #d7e0dc; font-size: 10px; font-weight: 700; letter-spacing: .05em; text-transform: uppercase; }
    .session-table-head span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .session-list { display: grid; gap: 8px; padding: 10px; background: #f7faf8; }
    .session-card { overflow: hidden; background: #fff; border: 1px solid #dce5e1; border-radius: 6px; }
    .session-summary { display: grid; width: 100%; grid-template-columns: 8px minmax(200px, 1.4fr) minmax(84px, .7fr) minmax(104px, .8fr) minmax(84px, .8fr) 64px 48px 86px 86px 18px; align-items: center; gap: 12px; padding: 12px 14px; color: #233834; background: #fff; border: 0; cursor: pointer; text-align: left; }
    .session-summary:hover { background: #fbfdfc; }
    .session-summary:focus-visible { outline: 2px solid #62a58d; outline-offset: -2px; }
    .session-indicator { width: 8px; height: 8px; border-radius: 50%; }
    .session-primary { display: grid; gap: 3px; min-width: 0; }
    .session-path { overflow: hidden; font: 13px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; text-overflow: ellipsis; white-space: nowrap; }
    .session-path b { color: #18755f; font-weight: 800; }
    .session-id { color: #7b8b87; font: 11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .session-cell { min-width: 0; overflow: hidden; color: #4a615b; font: 11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; text-overflow: ellipsis; white-space: nowrap; }
    .session-selector, .session-upstream { color: #327662; }
    .session-empty-cell { color: #a9b7b2; }
    .session-model { justify-self: start; max-width: 100%; overflow: hidden; padding: 1px 7px; color: #1c4a3d; background: #e5f1ec; border: 1px solid #bcd8cd; border-radius: 999px; font: 10px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; text-overflow: ellipsis; white-space: nowrap; }
    .session-state { justify-self: start; padding: 3px 7px; border-radius: 999px; font-size: 11px; font-weight: 700; white-space: nowrap; }
    .session-metric { display: grid; gap: 2px; text-align: right; }
    .session-metric small, .session-overview small { color: #82928e; font-size: 10px; font-weight: 700; letter-spacing: 0; text-transform: uppercase; }
    .session-metric strong, .session-overview strong { color: #314743; font: 12px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .session-chevron { width: 16px; height: 16px; color: #8b9a96; transition: transform .16s ease; }
    .session-card.expanded .session-chevron { transform: rotate(180deg); }
    .session-details { padding: 0 14px 14px; border-top: 1px solid #e8eeeb; background: #fbfdfc; }
    .session-overview { display: grid; grid-template-columns: repeat(auto-fit, minmax(110px, 1fr)); gap: 12px; padding: 12px 0; border-bottom: 1px solid #e7eeea; }
    .session-overview span { display: grid; gap: 3px; }
    .payload-preview { margin: 12px 0 0; overflow: hidden; border: 1px solid #dce7e2; border-radius: 5px; background: #12211e; }
    .payload-preview-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 10px; color: #b8ccc5; border-bottom: 1px solid #29423c; }
    .payload-preview-head h3 { margin: 0; color: #dceae5; font-size: 11px; font-weight: 800; letter-spacing: 0; text-transform: uppercase; }
    .payload-preview-head .payload-meta { overflow: hidden; color: #8eaaa1; font: 10px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; text-overflow: ellipsis; white-space: nowrap; }
    .payload-preview-head .payload-actions { display: flex; align-items: center; gap: 4px; flex-shrink: 0; }
    .payload-preview-head .payload-actions .icon-button { width: 26px; height: 26px; color: #8eaaa1; }
    .payload-preview-head .payload-actions .icon-button:hover { color: #dceae5; background: rgba(255, 255, 255, .08); }
    .payload-preview-head .payload-actions .icon-button.is-active { color: #cbe86b; }
    .payload-preview pre { max-height: 300px; padding: 11px; margin: 0; overflow: auto; color: #cae0d8; font: 11px/1.55 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
    .payload-preview-head { cursor: pointer; }
    .payload-preview.is-collapsed pre { display: none; }
    .payload-toggle-chevron { transition: transform .15s ease; }
    .payload-preview:not(.is-collapsed) .payload-toggle-chevron { transform: rotate(180deg); }
    .request-preview { border-color: #365d50; }
    .request-preview .payload-preview-head { background: #162a24; }
    .session-headers { padding-top: 12px; }
    .session-events { padding-top: 12px; }
    .session-headers h3, .session-events h3 { margin: 0 0 8px; color: #647773; font-size: 11px; font-weight: 800; letter-spacing: 0; text-transform: uppercase; }
    .header-list { max-height: 190px; padding: 0; margin: 0; overflow: auto; }
    .header-list div { display: grid; grid-template-columns: 145px minmax(0, 1fr); gap: 10px; padding: 5px 0; border-bottom: 1px solid #edf2ef; }
    .header-list dt { color: #6d807a; font: 11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .header-list dd { margin: 0; overflow-wrap: anywhere; color: #314641; font: 11px/1.45 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .request-list { padding: 0; margin: 0; list-style: none; }
    .request-list li { display: flex; justify-content: space-between; gap: 12px; padding: 7px 0; border-bottom: 1px solid #edf2ef; color: #30443f; font: 11px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .request-list li span:last-child { color: #748681; white-space: nowrap; }
    .gateway-events { display: grid; gap: 0; max-height: 190px; padding: 0; margin: 0; overflow: auto; list-style: none; }
    .gateway-events li { display: grid; grid-template-columns: 58px 82px minmax(0, 1fr); gap: 8px; padding: 6px 0; border-bottom: 1px solid #edf2ef; color: #334b45; font: 11px/1.4 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    .gateway-events time { color: #82938e; }
    .gateway-event-kind { color: #28765f; font-weight: 700; }
    .gateway-events-empty { display: block !important; color: #778984 !important; }
    .is-connecting { color: #9a6b1b; background: #fff5d8; }
    .session-indicator.is-connecting { background: #e6aa42; }
    .is-streaming { color: #21745f; background: #e3f5e9; }
    .session-indicator.is-streaming { background: #53a97f; box-shadow: 0 0 0 3px rgba(83, 169, 127, .12); }
    .is-completed { color: #506560; background: #edf2f0; }
    .session-indicator.is-completed { background: #9aa9a5; }
    .is-warning { color: #98691c; background: #fff4d8; }
    .session-indicator.is-warning { background: #e1a843; }
    .is-error { color: #a33e3e; background: #fff0ef; }
    .session-indicator.is-error { background: #d66a66; }
    .session-empty { padding: 28px; color: #7b8b87; font-size: 13px; text-align: center; }
    :root[data-theme="dark"] body { color: #dbe7e2; background: #0d1513; }
    :root[data-theme="dark"] .appbar { background: #112522; border-color: #294740; }
    :root[data-theme="dark"] .workspace, :root[data-theme="dark"] .telemetry { background: #14201d; border-color: #2b403a; box-shadow: 0 14px 30px rgba(0, 0, 0, .2); }
    :root[data-theme="dark"] .workspace-top { border-color: #293d38; }
    :root[data-theme="dark"] .section-title { color: #e3ede9; }
    :root[data-theme="dark"] .section-note, :root[data-theme="dark"] .summary { color: #8fa59e; }
    :root[data-theme="dark"] .icon-button { color: #b4c8c1; background: #1b2b27; border-color: #395047; }
    :root[data-theme="dark"] .icon-button:hover { color: #d4e8df; background: #253a34; border-color: #638d7f; }
    :root[data-theme="dark"] .icon-button.save { color: #17372f; background: #cbe86b; border-color: #cbe86b; }
    :root[data-theme="dark"] .icon-button.danger { color: #efa6a2; }
    :root[data-theme="dark"] .icon-button.danger:hover { color: #ffd0cc; background: #432728; border-color: #8b5551; }
    :root[data-theme="dark"] #status { color: #a9bbb5; }
    :root[data-theme="dark"] .table-scroll, :root[data-theme="dark"] .selector-list { background: #101a17; }
    :root[data-theme="dark"] th { color: #94aaa2; background: transparent; }
    :root[data-theme="dark"] th, :root[data-theme="dark"] td, :root[data-theme="dark"] .header-list div, :root[data-theme="dark"] .request-list li { border-color: #293d38; }
    :root[data-theme="dark"] tr[data-row] td { background: #172622; border-color: #30463f; }
    :root[data-theme="dark"] tr[data-row]:hover td, :root[data-theme="dark"] .session-summary:hover { background: #1a2a26; }
    :root[data-theme="dark"] .field-input, :root[data-theme="dark"] .auth-select { color: #dce9e4; }
    :root[data-theme="dark"] .field-input:hover, :root[data-theme="dark"] .auth-select:hover, :root[data-theme="dark"] .field-input:focus, :root[data-theme="dark"] .auth-select:focus { background: #1d302b; border-color: #527b6d; }
    :root[data-theme="dark"] .auth-select { color: #bbddd0; }
    :root[data-theme="dark"] .ms-trigger { color: #dce9e4; }
    :root[data-theme="dark"] .ms-trigger:hover, :root[data-theme="dark"] .ms-trigger:focus-visible { background: #1d302b; border-color: #527b6d; }
    :root[data-theme="dark"] .ms-chip { color: #bfe3d2; background: #1d3a31; border-color: #3c6658; }
    :root[data-theme="dark"] .ms-chip.is-stale { color: #f0c466; background: #453719; border-color: #7a5b20; }
    :root[data-theme="dark"] .ms-placeholder { color: #78958a; }
    :root[data-theme="dark"] .ms-chevron { color: #78958a; }
    :root[data-theme="dark"] .ms-menu { color: #dce9e4; background: #172622; border-color: #395047; box-shadow: 0 16px 32px rgba(0, 0, 0, .45); }
    :root[data-theme="dark"] .ms-option:hover { background: #1d302b; }
    :root[data-theme="dark"] .ms-empty { color: #8ba198; }
    :root[data-theme="dark"] .drag-handle { color: #78928a; }
    :root[data-theme="dark"] .telemetry-tabbar { background: #0e1916; border-color: #2b403a; }
    :root[data-theme="dark"] .telemetry-tab { color: #8ea69e; }
    :root[data-theme="dark"] .telemetry-tab:hover { color: #d2e3dc; background: #172923; }
    :root[data-theme="dark"] .telemetry-tab[aria-selected="true"] { color: #e7f0ec; background: #14201d; border-color: #2b403a; box-shadow: none; }
    :root[data-theme="dark"] .tab-connection { color: #78958a; }
    :root[data-theme="dark"] .selector-workspace { border-color: #2b403a; }
    :root[data-theme="dark"] .selector-table-head { color: #94aaa2; background: #101a17; }
    :root[data-theme="dark"] .selector-row { background: #172622; border-color: #30463f; }
    :root[data-theme="dark"] .selector-row:hover { background: #1a2a26; }
    :root[data-theme="dark"] .selector-empty { color: #8ba198; background: #172622; border-color: #405750; }
    :root[data-theme="dark"] .selector-matches { background: #101a17; border-color: #30463f; }
    :root[data-theme="dark"] .rule { background: #172622; border-color: #30463f; }
    :root[data-theme="dark"] .rule-switch span { background: #42574f; }
    :root[data-theme="dark"] .rule-switch span::after { background: #dfeae5; }
    :root[data-theme="dark"] .rule-switch input:checked + span { background: #4d8f79; }
    :root[data-theme="dark"] .rule-menu { background: #1a2a26; border-color: #30463f; box-shadow: 0 10px 24px rgba(0, 0, 0, .4); }
    :root[data-theme="dark"] .rule-menu button { color: #dce9e4; }
    :root[data-theme="dark"] .rule-menu button:hover { background: #243b34; color: #bbddd0; }
    :root[data-theme="dark"] .selector-no-rules { color: #8ba198; }
    :root[data-theme="dark"] .match-value-actions .rule-clear { color: #efa6a2; }
    :root[data-theme="dark"] .match-value-actions .rule-clear:hover { color: #ffd0cc; background: #432728; }
    :root[data-theme="dark"] .text-button { color: #8bc7a9; }
    :root[data-theme="dark"] .match-case-toggle.is-active { color: #17372f; background: #cbe86b; border-color: #cbe86b; }
    :root[data-theme="dark"] .session-list { background: #101a17; }
    :root[data-theme="dark"] .session-card, :root[data-theme="dark"] .session-summary { background: #172622; border-color: #30463f; color: #dce9e4; }
    :root[data-theme="dark"] .session-details { background: #13201c; border-color: #2a3f38; }
    :root[data-theme="dark"] .session-model { color: #bfe3d2; background: #1d3a31; border-color: #3c6658; }
    :root[data-theme="dark"] .session-path b { color: #79c9a7; }
    :root[data-theme="dark"] .session-id, :root[data-theme="dark"] .session-metric small, :root[data-theme="dark"] .session-overview small { color: #879e96; }
    :root[data-theme="dark"] .session-table-head { color: #94aaa2; background: #0e1916; border-color: #2b403a; }
    :root[data-theme="dark"] .session-selector, :root[data-theme="dark"] .session-upstream { color: #8bc7a9; }
    :root[data-theme="dark"] .session-cell { color: #a9bdb6; }
    :root[data-theme="dark"] .session-empty-cell { color: #5c716a; }
    :root[data-theme="dark"] .session-metric strong, :root[data-theme="dark"] .session-overview strong, :root[data-theme="dark"] .header-list dd, :root[data-theme="dark"] .request-list { color: #d1dfd9; }
    :root[data-theme="dark"] .session-headers h3, :root[data-theme="dark"] .session-events h3, :root[data-theme="dark"] .header-list dt, :root[data-theme="dark"] .gateway-events time { color: #93aaa1; }
    :root[data-theme="dark"] .gateway-events li { color: #c9d8d1; border-color: #293d38; }
    :root[data-theme="dark"] .gateway-event-kind { color: #7fcaab; }
    :root[data-theme="dark"] .is-completed { color: #b5c6c0; background: #263734; }
    :root[data-theme="dark"] .is-warning { color: #f0c466; background: #453719; }
    :root[data-theme="dark"] .is-error { color: #f3aaa6; background: #48292a; }
    :root[data-theme="dark"] .session-empty { color: #8ba198; }
    @media (max-width: 760px) { .shell { width: min(100% - 24px, 1220px); margin-top: 12px; } .appbar { align-items: flex-start; flex-direction: column; padding: 16px; border-radius: 8px; } .appbar-actions { width: 100%; justify-content: flex-end; } .workspace { border-top: 1px solid #d7e0dc; border-radius: 8px; margin-top: 12px; } .workspace-top { padding: 15px; } .section-note { display: none; } .summary { display: none; } .telemetry { margin-top: 12px; } .telemetry-tabbar { padding-inline: 8px; overflow-x: auto; } .telemetry-tab { padding-inline: 10px; } .selector-workspace { margin-top: 12px; } .selector-table-head { display: none; } .selector-row { grid-template-columns: 1fr; gap: 8px; padding: 12px 15px; } .selector-actions { justify-content: space-between; } .session-table-head { display: none; } .session-summary { grid-template-columns: 8px minmax(0, 1fr) auto 72px 18px; gap: 9px; } .session-metric { display: none; } .session-metric.session-transfer { display: grid; } .session-selector, .session-upstream, .session-model { display: none; } .header-list div { grid-template-columns: 105px minmax(0, 1fr); } .gateway-events li { grid-template-columns: 58px 76px minmax(0, 1fr) 34px 34px; } }
    @media (max-width: 760px) { .gateway-events li { grid-template-columns: 58px 76px minmax(0, 1fr); } }
  </style>
</head>
<body>
  <main class="shell">
    <header class="appbar">
      <div class="identity">
        <div class="brand-mark">AG</div>
        <div><p class="eyebrow">gateway control</p><h1>AGW</h1></div>
      </div>
      <div class="appbar-actions">
        <label class="debug-toggle" title="记录客户端请求头；日志可能包含认证信息">
          <input id="debug-toggle" type="checkbox"{{if .Debug}} checked{{end}}>
          <span class="switch"></span><span>Debug headers</span>
        </label>
        <span id="status" aria-live="polite"></span>
        <button class="icon-button" type="button" id="theme-toggle" title="切换到浅色主题" aria-label="切换到浅色主题"><i data-lucide="sun"></i></button>
        <button class="icon-button" type="button" title="刷新配置" aria-label="刷新配置" hx-get="/config" hx-target="#config-table" hx-swap="innerHTML"><i data-lucide="refresh-cw"></i></button>
        <button class="icon-button save" type="button" id="save" title="保存配置" aria-label="保存配置"><i data-lucide="save"></i></button>
      </div>
    </header>
    <section class="workspace" aria-labelledby="routing-title">
      <div class="workspace-top">
        <div><h2 class="section-title" id="routing-title">Upstream routing</h2><p class="section-note">按显示顺序重试，拖动行可调整优先级</p></div>
        <div class="section-actions"><div class="summary"><span class="summary-dot"></span><span id="upstream-count">加载中</span></div><button class="icon-button" type="button" id="add-upstream" title="新增上游" aria-label="新增上游"><i data-lucide="plus"></i></button></div>
      </div>
      <div class="table-scroll">
        <table>
          <thead><tr><th class="priority" scope="col">优先级</th><th class="name" scope="col">Name</th><th class="endpoint" scope="col">Upstream endpoint</th><th class="authentication" scope="col">Authentication</th><th class="app-selectors" scope="col">Compatible AppSelectors</th><th class="row-actions" scope="col">Actions</th></tr></thead>
          <tbody id="config-table" data-drop-zone hx-get="/config" hx-trigger="load" hx-swap="innerHTML"><tr><td colspan="6">正在加载上游配置...</td></tr></tbody>
        </table>
      </div>
    </section>
    <section class="workspace selector-workspace" aria-labelledby="selector-config-title">
      <div class="workspace-top">
        <div><h2 class="section-title" id="selector-config-title">AppSelector registry</h2><p class="section-note">按顺序匹配 request path、header 与 JSON body 字段；首个命中的 selector 决定后续 upstream retry 链。规则可单独启用 / 禁用，便于调试。</p></div>
        <div class="section-actions"><div class="summary"><span class="summary-dot"></span><span id="selector-count">加载中</span></div><button class="icon-button" type="button" id="add-selector" title="新增 AppSelector" aria-label="新增 AppSelector"><i data-lucide="plus"></i></button></div>
      </div>
      <div class="selector-table-head" role="row"><span>AppSelector</span><span>Rules</span><span>Actions</span></div>
      <div id="selector-list" class="selector-list" data-drop-zone>
        {{range .AppSelectors}}<div class="selector-row" data-selector data-draggable draggable="true"><div class="selector-name-cell"><span class="drag-handle" title="拖动排序"><i data-lucide="grip-vertical"></i></span><input class="field-input" data-selector-name value="{{.Name}}" placeholder="selector name" aria-label="AppSelector 名称"></div><div class="selector-matches selector-rules" data-selector-rules>{{range .Match.Path}}<div class="rule{{if not .RuleEnabled}} is-disabled{{end}}" data-rule data-rule-type="path" data-enabled="{{if .RuleEnabled}}true{{else}}false{{end}}"><span class="rule-kind">path</span><div class="rule-controls rule-controls-path"><select class="auth-select" data-rule-operator aria-label="匹配方式"><option value="exact"{{if eq .Operator "exact"}} selected{{end}}>exact</option><option value="prefix"{{if eq .Operator "prefix"}} selected{{end}}>prefix</option><option value="contains"{{if eq .Operator "contains"}} selected{{end}}>contains</option><option value="regex"{{if eq .Operator "regex"}} selected{{end}}>regex</option><option value="present"{{if eq .Operator "present"}} selected{{end}}>present</option></select><span class="match-value-field"><input class="field-input" data-rule-value value="{{.Value}}" placeholder="/v1/chat/completions" aria-label="匹配路径"{{if eq .Operator "present"}} disabled{{end}}></span></div><label class="rule-switch" title="启用 / 禁用规则"><input type="checkbox" data-rule-enabled{{if .RuleEnabled}} checked{{end}} aria-label="启用规则"><span></span></label><button class="icon-button rule-clear" type="button" data-rule-delete title="删除规则" aria-label="删除规则"><i data-lucide="x"></i></button></div>{{end}}{{range .Match.Headers}}<div class="rule{{if not .RuleEnabled}} is-disabled{{end}}" data-rule data-rule-type="header" data-case-sensitive="{{.CaseSensitive}}" data-enabled="{{if .RuleEnabled}}true{{else}}false{{end}}"><span class="rule-kind">header</span><div class="rule-controls rule-controls-header"><input class="field-input" data-rule-name value="{{.Name}}" placeholder="User-Agent" aria-label="匹配 header"><select class="auth-select" data-rule-operator aria-label="匹配方式"><option value="exact"{{if eq .Operator "exact"}} selected{{end}}>exact</option><option value="prefix"{{if eq .Operator "prefix"}} selected{{end}}>prefix</option><option value="contains"{{if eq .Operator "contains"}} selected{{end}}>contains</option><option value="regex"{{if eq .Operator "regex"}} selected{{end}}>regex</option><option value="present"{{if eq .Operator "present"}} selected{{end}}>present</option></select><span class="match-value-field"><input class="field-input" data-rule-value value="{{.Value}}" placeholder="匹配值" aria-label="匹配值"{{if eq .Operator "present"}} disabled{{end}}><span class="match-value-actions"><button class="icon-button match-case-toggle{{if .CaseSensitive}} is-active{{end}}" type="button" data-toggle-case title="区分大小写" aria-label="区分大小写" aria-pressed="{{.CaseSensitive}}"><i data-lucide="case-sensitive"></i></button></span></span></div><label class="rule-switch" title="启用 / 禁用规则"><input type="checkbox" data-rule-enabled{{if .RuleEnabled}} checked{{end}} aria-label="启用规则"><span></span></label><button class="icon-button rule-clear" type="button" data-rule-delete title="删除规则" aria-label="删除规则"><i data-lucide="x"></i></button></div>{{end}}{{range .Match.Body}}<div class="rule{{if not .RuleEnabled}} is-disabled{{end}}" data-rule data-rule-type="body" data-case-sensitive="{{.CaseSensitive}}" data-enabled="{{if .RuleEnabled}}true{{else}}false{{end}}"><span class="rule-kind">body</span><div class="rule-controls rule-controls-body"><input class="field-input" data-rule-name value="{{.Field}}" placeholder="model" aria-label="JSON 字段"><select class="auth-select" data-rule-operator aria-label="匹配方式"><option value="exact"{{if eq .Operator "exact"}} selected{{end}}>exact</option><option value="prefix"{{if eq .Operator "prefix"}} selected{{end}}>prefix</option><option value="contains"{{if eq .Operator "contains"}} selected{{end}}>contains</option><option value="regex"{{if eq .Operator "regex"}} selected{{end}}>regex</option><option value="present"{{if eq .Operator "present"}} selected{{end}}>present</option></select><span class="match-value-field"><input class="field-input" data-rule-value value="{{.Value}}" placeholder="匹配值" aria-label="匹配值"{{if eq .Operator "present"}} disabled{{end}}><span class="match-value-actions"><button class="icon-button match-case-toggle{{if .CaseSensitive}} is-active{{end}}" type="button" data-toggle-case title="区分大小写" aria-label="区分大小写" aria-pressed="{{.CaseSensitive}}"><i data-lucide="case-sensitive"></i></button></span></span></div><label class="rule-switch" title="启用 / 禁用规则"><input type="checkbox" data-rule-enabled{{if .RuleEnabled}} checked{{end}} aria-label="启用规则"><span></span></label><button class="icon-button rule-clear" type="button" data-rule-delete title="删除规则" aria-label="删除规则"><i data-lucide="x"></i></button></div>{{end}}{{range .Match.Query}}<div class="rule{{if not .RuleEnabled}} is-disabled{{end}}" data-rule data-rule-type="query" data-case-sensitive="{{.CaseSensitive}}" data-enabled="{{if .RuleEnabled}}true{{else}}false{{end}}"><span class="rule-kind">query</span><div class="rule-controls rule-controls-query"><input class="field-input" data-rule-name value="{{.Name}}" placeholder="api-version" aria-label="匹配 query 参数"><select class="auth-select" data-rule-operator aria-label="匹配方式"><option value="exact"{{if eq .Operator "exact"}} selected{{end}}>exact</option><option value="prefix"{{if eq .Operator "prefix"}} selected{{end}}>prefix</option><option value="contains"{{if eq .Operator "contains"}} selected{{end}}>contains</option><option value="regex"{{if eq .Operator "regex"}} selected{{end}}>regex</option><option value="present"{{if eq .Operator "present"}} selected{{end}}>present</option></select><span class="match-value-field"><input class="field-input" data-rule-value value="{{.Value}}" placeholder="匹配值" aria-label="匹配值"{{if eq .Operator "present"}} disabled{{end}}><span class="match-value-actions"><button class="icon-button match-case-toggle{{if .CaseSensitive}} is-active{{end}}" type="button" data-toggle-case title="区分大小写" aria-label="区分大小写" aria-pressed="{{.CaseSensitive}}"><i data-lucide="case-sensitive"></i></button></span></span></div><label class="rule-switch" title="启用 / 禁用规则"><input type="checkbox" data-rule-enabled{{if .RuleEnabled}} checked{{end}} aria-label="启用规则"><span></span></label><button class="icon-button rule-clear" type="button" data-rule-delete title="删除规则" aria-label="删除规则"><i data-lucide="x"></i></button></div>{{end}}{{range .Rewrite}}<div class="rule{{if not .RuleEnabled}} is-disabled{{end}}" data-rule data-rule-type="rewrite" data-enabled="{{if .RuleEnabled}}true{{else}}false{{end}}"><span class="rule-kind">rewrite</span><div class="rule-controls rule-controls-rewrite"><input class="field-input" data-rule-name value="{{.Field}}" placeholder="model" aria-label="要重写的字段"><input class="field-input" data-rule-value value="{{.Value}}" placeholder="gpt-5.6-luna" aria-label="重写后的值"></div><label class="rule-switch" title="启用 / 禁用规则"><input type="checkbox" data-rule-enabled{{if .RuleEnabled}} checked{{end}} aria-label="启用规则"><span></span></label><button class="icon-button rule-clear" type="button" data-rule-delete title="删除规则" aria-label="删除规则"><i data-lucide="x"></i></button></div>{{end}}<div class="selector-no-rules" data-rule-empty{{if .HasRules}} hidden{{end}}>No rules - matches all requests</div><div class="rule-add"><button class="text-button" type="button" data-add-rule title="添加规则" aria-label="添加规则"><i data-lucide="plus"></i>添加规则</button><div class="rule-menu" data-rule-menu hidden role="menu" aria-label="规则类型"><button type="button" data-rule-type-option="path">Path</button><button type="button" data-rule-type-option="header">Header</button><button type="button" data-rule-type-option="body">Body</button><button type="button" data-rule-type-option="query">Query</button><button type="button" data-rule-type-option="rewrite">Rewrite</button></div></div></div><div class="selector-actions"><button class="icon-button danger" type="button" data-delete-selector title="删除 AppSelector" aria-label="删除 AppSelector"><i data-lucide="trash-2"></i></button></div></div>{{else}}<div class="selector-empty">暂无 AppSelector；未配置 selector 时保持原有 upstream 顺序。</div>{{end}}
      </div>
    </section>
    <section class="telemetry" aria-labelledby="telemetry-title">
      <h2 class="sr-only" id="telemetry-title">Telemetry</h2>
      <div class="telemetry-tabbar" role="tablist" aria-label="观测视图"><button class="telemetry-tab" type="button" role="tab" id="sessions-tab" aria-selected="true" aria-controls="sessions-panel" data-telemetry-tab="sessions">Session journal</button><button class="telemetry-tab" type="button" role="tab" id="logs-tab" aria-selected="false" aria-controls="logs-panel" data-telemetry-tab="logs"><span class="live-dot"></span><span>Live request feed</span><span class="tab-connection">SSE connected</span></button></div>
      <div class="telemetry-panel" id="sessions-panel" role="tabpanel" aria-labelledby="sessions-tab"><div class="session-table-head" role="row"><span></span><span>Session</span><span>Selector</span><span>Upstream</span><span>Model</span><span>State</span><span>Status</span><span>Transfer</span><span>Duration</span><span></span></div><div id="session-list" class="session-list"></div></div>
      <div class="telemetry-panel" id="logs-panel" role="tabpanel" aria-labelledby="logs-tab" hidden><pre id="log-stream" hx-ext="sse" sse-connect="/logs" sse-swap="message" hx-swap="beforeend"></pre></div>
    </section>
  </main>
  <script>
    const table = document.getElementById('config-table');
    const selectorList = document.getElementById('selector-list');
    const status = document.getElementById('status');
    const saveButton = document.getElementById('save');
    const themeToggle = document.getElementById('theme-toggle');
    const sessionList = document.getElementById('session-list');
    const expandedSessions = new Set();
    const requestPayloadCache = new Map();
    let dragged, dragContainer, dropIndicator, dragGhost;
    function renderIcons(scope) { if (window.lucide) window.lucide.createIcons({root: scope || document, attrs: {'stroke-width': 1.8}}); }
    function setTheme(theme) { document.documentElement.dataset.theme = theme; try { localStorage.setItem('agw-theme', theme); } catch (_) {} const isDark = theme === 'dark'; themeToggle.title = isDark ? '切换到浅色主题' : '切换到深色主题'; themeToggle.setAttribute('aria-label', themeToggle.title); themeToggle.innerHTML = '<i data-lucide="' + (isDark ? 'sun' : 'moon') + '"></i>'; renderIcons(themeToggle); }
    function setTelemetryView(view, focus) { document.querySelectorAll('[data-telemetry-tab]').forEach(tab => { const active = tab.dataset.telemetryTab === view; tab.setAttribute('aria-selected', String(active)); document.getElementById(tab.getAttribute('aria-controls')).hidden = !active; if (active && focus) tab.focus(); }); }
    function replacePayloadText(target, text) {
      // Replacing textContent resets the scroll position of the <pre>; restore
      // it so live updates never yank the reader back to the top.
      const scrollTop = target.scrollTop;
      target.textContent = text;
      target.scrollTop = scrollTop;
    }
    function renderPayload(target) {
      const raw = target.dataset.raw || '';
      const preview = target.closest('.payload-preview');
      const pretty = preview && preview.querySelector('[data-payload-pretty]');
      if (pretty && pretty.classList.contains('is-active')) {
        try { replacePayloadText(target, JSON.stringify(JSON.parse(raw), null, 2)); return; } catch (_) {}
      }
      replacePayloadText(target, raw);
    }
    function copyText(text) {
      if (navigator.clipboard && navigator.clipboard.writeText) return navigator.clipboard.writeText(text);
      return new Promise(function (resolve, reject) {
        const area = document.createElement('textarea');
        area.value = text;
        area.style.position = 'fixed';
        area.style.opacity = '0';
        document.body.append(area);
        area.select();
        try { document.execCommand('copy') ? resolve() : reject(new Error('copy failed')); } catch (error) { reject(error); } finally { area.remove(); }
      });
    }
    function hydratePayload(target, sessionID) {
      const kind = target.dataset.sessionPayload;
      const cacheKey = sessionID + ':' + kind;
      if (kind === 'request' && requestPayloadCache.has(cacheKey)) {
        target.dataset.raw = requestPayloadCache.get(cacheKey);
        renderPayload(target);
        return;
      }
      fetch('/sessions/' + encodeURIComponent(sessionID) + '/' + kind)
        .then(response => { if (!response.ok) throw new Error('payload ' + response.status); return response.text(); })
        .then(payload => { target.dataset.raw = payload; renderPayload(target); if (kind === 'request') requestPayloadCache.set(cacheKey, payload); })
        .catch(() => {});
    }
    function hydrateSessionPayloads(card) {
      const sessionID = card.dataset.sessionId;
      card.querySelectorAll('[data-session-payload]').forEach(target => {
        const preview = target.closest('.payload-preview');
        if (preview && preview.classList.contains('is-collapsed')) return;
        hydratePayload(target, sessionID);
      });
    }
    function setSessionExpanded(card, expanded) {
      const details = card.querySelector('.session-details');
      card.querySelector('[data-session-toggle]').setAttribute('aria-expanded', String(expanded));
      details.hidden = !expanded;
      card.classList.toggle('expanded', expanded);
      if (expanded) {
        expandedSessions.add(card.dataset.sessionId);
        hydrateSessionPayloads(card);
      } else {
        expandedSessions.delete(card.dataset.sessionId);
        requestPayloadCache.delete(card.dataset.sessionId + ':request');
        card.querySelectorAll('.payload-preview').forEach(preview => {
          preview.classList.add('is-collapsed');
          const head = preview.querySelector('[data-payload-toggle]');
          if (head) head.setAttribute('aria-expanded', 'false');
          const target = preview.querySelector('[data-session-payload]');
          if (target) delete target.dataset.raw;
        });
      }
    }
    const metricTextInterval = 1000; // ms; how often live metric text (transfer/duration) may be written
    const pendingTextUpdates = new Map();
    let textFlushTimer = null;
    function scheduleTextUpdate(el, text) {
      if (!el || el.textContent === text) return;
      pendingTextUpdates.set(el, text);
      if (!textFlushTimer) textFlushTimer = setTimeout(flushTextUpdates, metricTextInterval);
    }
    function flushTextUpdates() {
      textFlushTimer = null;
      pendingTextUpdates.forEach((text, el) => { if (el.isConnected && el.textContent !== text) el.textContent = text; });
      pendingTextUpdates.clear();
    }
    function syncSessionSummary(oldSummary, newSummary) {
      const indicator = oldSummary.querySelector('.session-indicator');
      const nextIndicator = newSummary.querySelector('.session-indicator');
      if (indicator && nextIndicator && indicator.className !== nextIndicator.className) indicator.className = nextIndicator.className;
      const state = oldSummary.querySelector('.session-state');
      const nextState = newSummary.querySelector('.session-state');
      if (state && nextState) {
        if (state.className !== nextState.className) state.className = nextState.className;
        if (state.textContent !== nextState.textContent) state.textContent = nextState.textContent;
      }
      for (const selector of ['.session-path', '.session-id', '.session-selector', '.session-upstream', '.session-model']) {
        const el = oldSummary.querySelector(selector);
        const next = newSummary.querySelector(selector);
        if (el && next && el.textContent !== next.textContent) el.textContent = next.textContent;
      }
      const metrics = [...oldSummary.querySelectorAll('.session-metric')];
      const nextMetrics = [...newSummary.querySelectorAll('.session-metric')];
      metrics.forEach((metric, i) => {
        const next = nextMetrics[i];
        if (!next) return;
        const strong = metric.querySelector('strong');
        const nextStrong = next.querySelector('strong');
        if (strong && nextStrong) scheduleTextUpdate(strong, nextStrong.textContent);
        const small = metric.querySelector('small');
        const nextSmall = next.querySelector('small');
        if (small && nextSmall) scheduleTextUpdate(small, nextSmall.textContent);
      });
    }
    function syncSessionOverview(oldOverview, newOverview) {
      const items = [...oldOverview.querySelectorAll('span')];
      const nextItems = [...newOverview.querySelectorAll('span')];
      items.forEach((item, i) => {
        const next = nextItems[i];
        if (!next) return;
        const strong = item.querySelector('strong');
        const nextStrong = next.querySelector('strong');
        if (strong && nextStrong) {
          if (strong.className !== nextStrong.className) strong.className = nextStrong.className;
          scheduleTextUpdate(strong, nextStrong.textContent);
        }
        const small = item.querySelector('small');
        const nextSmall = next.querySelector('small');
        if (small && nextSmall) scheduleTextUpdate(small, nextSmall.textContent);
      });
    }
    function summaryStructure(summary) { return [...summary.children].map(el => el.className).join('|'); }
    function withScrollPreserved(node, fn) {
      const scrollables = [...node.querySelectorAll('pre[data-session-payload], .header-list, .gateway-events')];
      const positions = scrollables.map(el => el.scrollTop);
      fn();
      scrollables.forEach((el, i) => { el.scrollTop = positions[i]; });
    }
    function updateSessionCard(oldCard, newCard) {
      const oldSummary = oldCard.querySelector('.session-summary');
      const newSummary = newCard.querySelector('.session-summary');
      if (oldSummary && newSummary && oldSummary.outerHTML !== newSummary.outerHTML) {
        if (summaryStructure(oldSummary) === summaryStructure(newSummary)) syncSessionSummary(oldSummary, newSummary);
        else { oldSummary.replaceWith(newSummary); renderIcons(newSummary); }
      }
      const oldDetails = oldCard.querySelector('.session-details');
      const newDetails = newCard.querySelector('.session-details');
      if (!oldDetails || !newDetails) return;
      const oldOverview = oldDetails.querySelector('.session-overview');
      const newOverview = newDetails.querySelector('.session-overview');
      if (oldOverview && newOverview && oldOverview.outerHTML !== newOverview.outerHTML) syncSessionOverview(oldOverview, newOverview);
      const oldHeaders = oldDetails.querySelector('.session-headers');
      const newHeaders = newDetails.querySelector('.session-headers');
      if (oldHeaders && newHeaders && oldHeaders.outerHTML !== newHeaders.outerHTML) oldHeaders.replaceWith(newHeaders);
      const oldEvents = oldDetails.querySelector('.session-events');
      const newEvents = newDetails.querySelector('.session-events');
      if (oldEvents && newEvents && oldEvents.outerHTML !== newEvents.outerHTML) oldEvents.replaceWith(newEvents);
      const newRequest = newDetails.querySelector('.request-preview');
      const oldRequest = oldDetails.querySelector('.request-preview');
      if (newRequest && !oldRequest) { oldDetails.querySelector('.session-headers').insertAdjacentElement('afterend', newRequest); renderIcons(newRequest); hydrateSessionPayloads(oldCard); }
      else if (newRequest && oldRequest) { const oldHead = oldRequest.querySelector('.payload-preview-head span'); const newHead = newRequest.querySelector('.payload-preview-head span'); if (oldHead && newHead && oldHead.textContent !== newHead.textContent) oldHead.textContent = newHead.textContent; }
      const newResponse = newDetails.querySelector('.response-preview');
      const oldResponse = oldDetails.querySelector('.response-preview');
      if (newResponse && !oldResponse) { const anchor = oldDetails.querySelector('.request-preview') || oldDetails.querySelector('.session-headers'); anchor.insertAdjacentElement('afterend', newResponse); renderIcons(newResponse); hydrateSessionPayloads(oldCard); }
      else if (newResponse && oldResponse) { const oldHead = oldResponse.querySelector('.payload-preview-head span'); const newHead = newResponse.querySelector('.payload-preview-head span'); if (oldHead && newHead && oldHead.textContent !== newHead.textContent) oldHead.textContent = newHead.textContent; }
      if (oldCard.classList.contains('expanded')) { const toggle = oldCard.querySelector('[data-session-toggle]'); if (toggle) toggle.setAttribute('aria-expanded', 'true'); }
    }
    function reconcileSessionCards(html) {
      const doc = new DOMParser().parseFromString(html, 'text/html');
      const incoming = [...doc.querySelectorAll('.session-card')];
      const incomingIds = new Set(incoming.map(card => card.dataset.sessionId));
      const existingById = new Map([...sessionList.querySelectorAll('.session-card')].map(card => [card.dataset.sessionId, card]));
      // Moving a card node resets the scroll position of its scrollable
      // children (e.g. the response preview), so only move cards that are
      // actually out of order and insert new cards at their final slot.
      const cards = [...sessionList.querySelectorAll('.session-card')];
      incoming.forEach((card, index) => {
        const oldCard = existingById.get(card.dataset.sessionId);
        if (oldCard) {
          updateSessionCard(oldCard, card);
          const currentIndex = cards.indexOf(oldCard);
          if (currentIndex === index) return;
          let reference;
          if (currentIndex > index) reference = index < cards.length ? cards[index] : null;
          else reference = index + 1 < cards.length ? cards[index + 1] : null;
          withScrollPreserved(oldCard, () => sessionList.insertBefore(oldCard, reference || null));
          cards.splice(currentIndex, 1);
          cards.splice(index, 0, oldCard);
          return;
        }
        const reference = index < cards.length ? cards[index] : null;
        sessionList.insertBefore(card, reference || null);
        cards.splice(index, 0, card);
        renderIcons(card);
      });
      sessionList.querySelectorAll('.session-card').forEach(card => { if (!incomingIds.has(card.dataset.sessionId)) card.remove(); });
      const empty = sessionList.querySelector('.session-empty');
      if (incoming.length) { if (empty) empty.remove(); }
      else if (!sessionList.querySelector('.session-card')) { const el = doc.querySelector('.session-empty'); if (el) sessionList.append(el); }
    }
    function refreshResponsePreviews() {
      sessionList.querySelectorAll('.session-card.expanded .response-preview:not(.is-collapsed) [data-session-payload="response"]').forEach(pre => {
        const card = pre.closest('.session-card');
        const indicator = card.querySelector('.session-indicator');
        if (indicator && /is-(completed|warning|error)/.test(indicator.className)) return;
        fetch('/sessions/' + encodeURIComponent(card.dataset.sessionId) + '/response').then(response => response.text()).then(payload => { if (pre.dataset.raw !== payload) { pre.dataset.raw = payload; renderPayload(pre); } }).catch(() => {});
      });
    }
    function updateSummary() { document.getElementById('upstream-count').textContent = table.querySelectorAll('tr[data-row]').length + ' upstreams'; }
    function updateSelectorSummary() { document.getElementById('selector-count').textContent = selectorList.querySelectorAll('[data-selector]').length + ' selectors'; }
    function ensureDuplicateButtons(scope) { scope.querySelectorAll('tr[data-row]').forEach(row => { const actions = row.querySelector('.row-actions'); if (!actions || actions.querySelector('[data-duplicate-row]')) return; const remove = actions.querySelector('[data-delete-row]'); remove.insertAdjacentHTML('beforebegin', '<button class="icon-button" type="button" data-duplicate-row title="复制 upstream" aria-label="复制 upstream"><i data-lucide="copy"></i></button>'); }); scope.querySelectorAll('[data-selector]').forEach(row => { const actions = row.querySelector('.selector-actions'); if (!actions || actions.querySelector('[data-duplicate-selector]')) return; const remove = actions.querySelector('[data-delete-selector]'); remove.insertAdjacentHTML('beforebegin', '<button class="icon-button" type="button" data-duplicate-selector title="复制 AppSelector" aria-label="复制 AppSelector"><i data-lucide="copy"></i></button>'); }); }
    function newRow() { return '<tr data-row draggable="true"><td class="priority"><span class="drag-handle" title="拖动排序"><i data-lucide="grip-vertical"></i></span></td><td><input class="field-input" data-name value="" placeholder="名称" aria-label="上游名称"></td><td><input class="field-input" data-url value="https://example.com/v1" aria-label="上游地址"></td><td><div class="auth"><select class="auth-select" data-auth-type aria-label="认证类型"><option value="none" selected>none</option><option value="basic">basic</option><option value="bearer">bearer</option></select><span class="auth-value"><input class="field-input" data-auth-value type="password" value="" aria-label="认证值"></span><button class="icon-button" type="button" data-toggle-password title="显示认证值" aria-label="显示认证值"><i data-lucide="eye"></i></button></div></td><td class="app-selectors"><div class="multi-select" data-multi-select><input type="hidden" data-app-selectors value=""><div class="ms-trigger" data-ms-trigger role="button" tabindex="0" aria-haspopup="listbox" aria-expanded="false" aria-label="兼容的 AppSelector"><span class="ms-chips" data-ms-chips></span><i data-lucide="chevron-down" class="ms-chevron"></i></div><div class="ms-menu" data-ms-menu hidden role="listbox" aria-multiselectable="true"></div></div></td><td class="row-actions"><button class="icon-button danger" type="button" data-delete-row title="删除上游" aria-label="删除上游"><i data-lucide="trash-2"></i></button></td></tr>'; }
    function registeredSelectorNames() { return [...selectorList.querySelectorAll('[data-selector]')].map(row => row.querySelector('[data-selector-name]').value.trim()).filter(Boolean); }
    function parseSelectorList(value) { return String(value || '').split(',').map(item => item.trim()).filter(Boolean); }
    function setMultiSelectValue(ms, names) { ms.querySelector('[data-app-selectors]').value = [...new Set(names)].join(', '); renderMultiSelect(ms); }
    function renderMultiSelect(scope) {
      const roots = scope.matches && scope.matches('[data-multi-select]') ? [scope] : [...scope.querySelectorAll('[data-multi-select]')];
      roots.forEach(function (ms) {
        const hidden = ms.querySelector('[data-app-selectors]');
        const chips = ms.querySelector('[data-ms-chips]');
        const menu = ms.querySelector('[data-ms-menu]');
        const trigger = ms.querySelector('[data-ms-trigger]');
        const registered = registeredSelectorNames();
        const selected = parseSelectorList(hidden.value);
        const allNames = [...new Set(registered.concat(selected))];
        menu.innerHTML = '';
        if (!allNames.length) {
          const empty = document.createElement('div');
          empty.className = 'ms-empty';
          empty.textContent = '暂无 AppSelector，请先在下方注册';
          menu.append(empty);
        }
        allNames.forEach(function (name) {
          const label = document.createElement('label');
          label.className = 'ms-option' + (registered.includes(name) ? '' : ' is-stale');
          label.title = registered.includes(name) ? '' : '该 AppSelector 已不存在';
          const check = document.createElement('input');
          check.type = 'checkbox';
          check.dataset.msCheck = '';
          check.value = name;
          check.checked = selected.includes(name);
          const span = document.createElement('span');
          span.textContent = name;
          label.append(check, span);
          menu.append(label);
        });
        chips.innerHTML = '';
        if (!selected.length) {
          const placeholder = document.createElement('span');
          placeholder.className = 'ms-placeholder';
          placeholder.textContent = registered.length ? '未选择（不参与路由）' : '未选择';
          chips.append(placeholder);
        } else {
          selected.forEach(function (name) {
            const chip = document.createElement('span');
            chip.className = 'ms-chip' + (registered.includes(name) ? '' : ' is-stale');
            chip.dataset.msChip = name;
            const text = document.createElement('span');
            text.textContent = name;
            const remove = document.createElement('button');
            remove.type = 'button';
            remove.dataset.msRemove = '';
            remove.title = '移除 ' + name;
            remove.setAttribute('aria-label', '移除 ' + name);
            remove.innerHTML = '<i data-lucide="x"></i>';
            chip.append(text, remove);
            chips.append(chip);
          });
        }
        trigger.title = !selected.length && registered.length ? '未绑定任何 AppSelector：配置了 selector 时该上游不会参与路由' : '选择兼容的 AppSelector';
        renderIcons(chips);
      });
    }
    function openMultiSelect(ms) {
      closeMultiSelects();
      const trigger = ms.querySelector('[data-ms-trigger]');
      const menu = ms.querySelector('[data-ms-menu]');
      const rect = trigger.getBoundingClientRect();
      menu.hidden = false;
      trigger.setAttribute('aria-expanded', 'true');
      const width = Math.min(Math.max(rect.width, 240), window.innerWidth - 16);
      menu.style.width = width + 'px';
      menu.style.left = Math.max(8, Math.min(rect.left, window.innerWidth - width - 8)) + 'px';
      menu.style.top = Math.min(rect.bottom + 4, window.innerHeight - 40) + 'px';
      menu.style.maxHeight = Math.max(120, Math.min(260, window.innerHeight - parseFloat(menu.style.top) - 12)) + 'px';
    }
    function closeMultiSelects() {
      document.querySelectorAll('[data-multi-select]').forEach(function (ms) {
        const menu = ms.querySelector('[data-ms-menu]');
        ms.querySelector('[data-ms-trigger]').setAttribute('aria-expanded', 'false');
        menu.hidden = true;
        menu.style.width = '';
        menu.style.left = '';
        menu.style.top = '';
        menu.style.maxHeight = '';
      });
    }
    const ruleOperators = '<option value="exact" selected>exact</option><option value="prefix">prefix</option><option value="contains">contains</option><option value="regex">regex</option><option value="present">present</option>';
    function ruleSwitch() { return '<label class="rule-switch" title="启用 / 禁用规则"><input type="checkbox" data-rule-enabled checked aria-label="启用规则"><span></span></label>'; }
    function ruleDelete() { return '<button class="icon-button rule-clear" type="button" data-rule-delete title="删除规则" aria-label="删除规则"><i data-lucide="x"></i></button>'; }
    function ruleRow(type) {
      const controls = {
        path: '<div class="rule-controls rule-controls-path"><select class="auth-select" data-rule-operator aria-label="匹配方式">' + ruleOperators + '</select><span class="match-value-field"><input class="field-input" data-rule-value value="" placeholder="/v1/chat/completions" aria-label="匹配路径"></span></div>',
        header: '<div class="rule-controls rule-controls-header"><input class="field-input" data-rule-name value="" placeholder="User-Agent" aria-label="匹配 header"><select class="auth-select" data-rule-operator aria-label="匹配方式">' + ruleOperators + '</select><span class="match-value-field"><input class="field-input" data-rule-value value="" placeholder="匹配值" aria-label="匹配值"><span class="match-value-actions"><button class="icon-button match-case-toggle" type="button" data-toggle-case title="区分大小写" aria-label="区分大小写" aria-pressed="false"><i data-lucide="case-sensitive"></i></button></span></span></div>',
        query: '<div class="rule-controls rule-controls-query"><input class="field-input" data-rule-name value="" placeholder="api-version" aria-label="匹配 query 参数"><select class="auth-select" data-rule-operator aria-label="匹配方式">' + ruleOperators + '</select><span class="match-value-field"><input class="field-input" data-rule-value value="" placeholder="匹配值" aria-label="匹配值"><span class="match-value-actions"><button class="icon-button match-case-toggle" type="button" data-toggle-case title="区分大小写" aria-label="区分大小写" aria-pressed="false"><i data-lucide="case-sensitive"></i></button></span></span></div>',
        body: '<div class="rule-controls rule-controls-body"><input class="field-input" data-rule-name value="" placeholder="model" aria-label="JSON 字段"><select class="auth-select" data-rule-operator aria-label="匹配方式">' + ruleOperators + '</select><span class="match-value-field"><input class="field-input" data-rule-value value="" placeholder="匹配值" aria-label="匹配值"><span class="match-value-actions"><button class="icon-button match-case-toggle" type="button" data-toggle-case title="区分大小写" aria-label="区分大小写" aria-pressed="false"><i data-lucide="case-sensitive"></i></button></span></span></div>',
        rewrite: '<div class="rule-controls rule-controls-rewrite"><input class="field-input" data-rule-name value="" placeholder="model" aria-label="要重写的字段"><input class="field-input" data-rule-value value="" placeholder="gpt-5.6-luna" aria-label="重写后的值"></div>'
      };
      return '<div class="rule" data-rule data-rule-type="' + type + '" data-case-sensitive="false" data-enabled="true"><span class="rule-kind">' + type + '</span>' + controls[type] + ruleSwitch() + ruleDelete() + '</div>';
    }
    function ruleEmpty() { return '<div class="selector-no-rules" data-rule-empty>No rules - matches all requests</div>'; }
    function ruleAddBlock() { return '<div class="rule-add"><button class="text-button" type="button" data-add-rule title="添加规则" aria-label="添加规则"><i data-lucide="plus"></i>添加规则</button><div class="rule-menu" data-rule-menu hidden role="menu" aria-label="规则类型"><button type="button" data-rule-type-option="path">Path</button><button type="button" data-rule-type-option="header">Header</button><button type="button" data-rule-type-option="body">Body</button><button type="button" data-rule-type-option="query">Query</button><button type="button" data-rule-type-option="rewrite">Rewrite</button></div></div>'; }
    function selectorRow() { return '<div class="selector-row" data-selector data-draggable draggable="true"><div class="selector-name-cell"><span class="drag-handle" title="拖动排序"><i data-lucide="grip-vertical"></i></span><input class="field-input" data-selector-name value="" placeholder="selector name" aria-label="AppSelector 名称"></div><div class="selector-matches selector-rules" data-selector-rules>' + ruleEmpty() + ruleAddBlock() + '</div><div class="selector-actions"><button class="icon-button danger" type="button" data-delete-selector title="删除 AppSelector" aria-label="删除 AppSelector"><i data-lucide="trash-2"></i></button></div></div>'; }
    function draggableRows(container) { return [...container.children].filter(node => node.matches && node.matches('[data-draggable], tr[data-row]')); }
    function createDropIndicator() { const indicator = document.createElement(dragged.tagName === 'TR' ? 'tr' : 'div'); indicator.className = 'drop-indicator'; if (dragged.tagName === 'TR') { const cell = document.createElement('td'); cell.colSpan = 6; const label = document.createElement('span'); label.textContent = '松手后放到这里'; cell.append(label); indicator.append(cell); } else { indicator.textContent = '松手后放到这里'; } return indicator; }
    function clearDropPreview() { if (dropIndicator) dropIndicator.remove(); dropIndicator = null; if (dragGhost) dragGhost.remove(); dragGhost = null; }
    function capturePositions(container) { return new Map(draggableRows(container).map(row => [row, row.getBoundingClientRect().top])); }
    function animateReorder(container, positions) { draggableRows(container).forEach(row => { const before = positions.get(row); const after = row.getBoundingClientRect().top; const offset = before == null ? 0 : before - after; if (offset) { row.style.transition = 'none'; row.style.transform = 'translateY(' + offset + 'px)'; requestAnimationFrame(() => { row.style.transition = ''; row.style.transform = ''; }); } }); }
    function placeDropIndicator(container, before) { if (!dropIndicator) dropIndicator = createDropIndicator(); container.insertBefore(dropIndicator, before); }
    function makeDragGhost(event, row) { if (!event.dataTransfer) return; dragGhost = document.createElement('div'); dragGhost.className = 'drag-ghost'; dragGhost.textContent = row.tagName === 'TR' ? '移动 upstream' : '移动 AppSelector'; document.body.append(dragGhost); event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', 'agw-reorder'); event.dataTransfer.setDragImage(dragGhost, 18, 16); }
    document.addEventListener('dragstart', function (event) { const row = event.target.closest('[data-draggable], tr[data-row]'); if (!row || event.target.closest('input, select, button, label')) { event.preventDefault(); return; } dragged = row; dragContainer = row.parentNode; row.classList.add('dragging'); makeDragGhost(event, row); });
    document.addEventListener('dragover', function (event) { if (!dragged) return; const row = event.target.closest('[data-draggable], tr[data-row]'); const zone = event.target.closest('[data-drop-zone]'); const container = row ? row.parentNode : zone; if (!container || container !== dragContainer) return; event.preventDefault(); if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'; if (event.target.closest('.drop-indicator')) return; if (!row || row === dragged) { const rows = draggableRows(container).filter(item => item !== dragged); placeDropIndicator(container, rows.length ? rows[rows.length - 1].nextSibling : null); return; } const box = row.getBoundingClientRect(); placeDropIndicator(container, event.clientY < box.top + box.height / 2 ? row : row.nextSibling); });
    document.addEventListener('drop', function (event) { const zone = event.target.closest('[data-drop-zone]'); if (!dragged || !zone || zone !== dragContainer || !dropIndicator) return; event.preventDefault(); const positions = capturePositions(dragContainer); dragContainer.insertBefore(dragged, dropIndicator); dropIndicator.remove(); dropIndicator = null; animateReorder(dragContainer, positions); dragged.classList.remove('dragging'); dragged.classList.add('reorder-settled'); setTimeout(() => dragged && dragged.classList.remove('reorder-settled'), 560); });
    document.addEventListener('dragend', function () { if (dragged) dragged.classList.remove('dragging'); clearDropPreview(); dragged = null; dragContainer = null; });
    document.addEventListener('change', function (event) {
      const operator = event.target.closest('[data-rule-operator]');
      if (operator) { const rule = operator.closest('[data-rule]'); const value = rule.querySelector('[data-rule-value]'); const present = operator.value === 'present'; value.disabled = present; if (present) value.value = ''; return; }
      const enabled = event.target.closest('[data-rule-enabled]');
      if (enabled) { const rule = enabled.closest('[data-rule]'); const on = enabled.checked; rule.dataset.enabled = String(on); rule.classList.toggle('is-disabled', !on); }
    });
    saveButton.addEventListener('click', async function () {
      const upstreams = [...table.querySelectorAll('tr[data-row]')].map(row => ({name: row.querySelector('[data-name]').value.trim(), url: row.querySelector('[data-url]').value.trim(), appSelectors: row.querySelector('[data-app-selectors]').value.split(',').map(value => value.trim()).filter(Boolean), authorization: {type: row.querySelector('[data-auth-type]').value, value: row.querySelector('[data-auth-value]').value}}));
      const appSelectors = [...document.querySelectorAll('[data-selector]')].map(row => { const path = [], query = [], headers = [], body = [], rewrite = []; row.querySelectorAll('[data-rule]').forEach(rule => { const type = rule.dataset.ruleType; const enabled = rule.dataset.enabled === 'true' ? undefined : false; const operator = (rule.querySelector('[data-rule-operator]') || {}).value || ''; const value = (rule.querySelector('[data-rule-value]') || {}).value || ''; const name = (rule.querySelector('[data-rule-name]') || {}).value.trim() || ''; if (type === 'path') path.push({operator, value, enabled}); else if (type === 'query') query.push({name, operator, value, caseSensitive: rule.dataset.caseSensitive === 'true', enabled}); else if (type === 'header') headers.push({name, operator, value, caseSensitive: rule.dataset.caseSensitive === 'true', enabled}); else if (type === 'body') body.push({field: name, operator, value, caseSensitive: rule.dataset.caseSensitive === 'true', enabled}); else if (type === 'rewrite') rewrite.push({field: name, value, enabled}); }); return {name: row.querySelector('[data-selector-name]').value.trim(), match: {path, query, headers, body}, rewrite}; });
      const registeredNames = new Set(registeredSelectorNames());
      const staleNames = [...new Set(upstreams.flatMap(upstream => upstream.appSelectors.filter(name => name && !registeredNames.has(name))))];
      if (staleNames.length) {
        status.className = 'error';
        status.textContent = '无法保存：以下 AppSelector 不存在：' + staleNames.join('、') + '（请先在 registry 中创建，或移除对应选择）';
        return;
      }
      status.className = ''; status.textContent = '保存中'; saveButton.disabled = true;
      try {
        const response = await fetch('/config', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({debug: document.getElementById('debug-toggle').checked, appSelectors, upstreams})});
        if (!response.ok) throw new Error(await response.text());
        status.className = 'success'; status.textContent = '已保存'; htmx.trigger(table, 'load');
      } catch (error) { status.className = 'error'; status.textContent = error.message || '保存失败'; }
      finally { saveButton.disabled = false; }
    });
    document.getElementById('add-upstream').addEventListener('click', function () { table.insertAdjacentHTML('beforeend', newRow()); ensureDuplicateButtons(table); renderMultiSelect(table); renderIcons(table); updateSummary(); });
    document.getElementById('add-selector').addEventListener('click', function () { const empty = selectorList.querySelector('.selector-empty'); if (empty) empty.remove(); selectorList.insertAdjacentHTML('beforeend', selectorRow()); ensureDuplicateButtons(selectorList); renderIcons(selectorList); updateSelectorSummary(); renderMultiSelect(document); });
    themeToggle.addEventListener('click', function () { setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'); });
    document.addEventListener('click', function (event) {
      const removeChip = event.target.closest('[data-ms-remove]');
      if (removeChip) { const ms = removeChip.closest('[data-multi-select]'); const chipName = removeChip.closest('[data-ms-chip]').dataset.msChip; setMultiSelectValue(ms, parseSelectorList(ms.querySelector('[data-app-selectors]').value).filter(name => name !== chipName)); return; }
      const trigger = event.target.closest('[data-ms-trigger]');
      if (trigger) { const ms = trigger.closest('[data-multi-select]'); const menu = ms.querySelector('[data-ms-menu]'); if (menu.hidden) openMultiSelect(ms); else closeMultiSelects(); return; }
      if (!event.target.closest('[data-ms-menu]')) closeMultiSelects();
    });
    document.addEventListener('change', function (event) {
      const check = event.target.closest('[data-ms-check]');
      if (check) { const ms = check.closest('[data-multi-select]'); const value = check.value; const names = new Set(parseSelectorList(ms.querySelector('[data-app-selectors]').value)); check.checked ? names.add(value) : names.delete(value); setMultiSelectValue(ms, [...names]); const next = [...ms.querySelectorAll('[data-ms-check]')].find(el => el.value === value); if (next) next.focus(); return; }
      if (event.target.closest('[data-selector-name]')) renderMultiSelect(document);
    });
    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape') { closeMultiSelects(); document.querySelectorAll('[data-rule-menu]').forEach(menu => menu.hidden = true); return; }
      const trigger = event.target.closest('[data-ms-trigger]');
      if (!trigger) return;
      if (['Enter', ' ', 'ArrowDown'].includes(event.key)) {
        event.preventDefault();
        const ms = trigger.closest('[data-multi-select]');
        if (ms.querySelector('[data-ms-menu]').hidden) openMultiSelect(ms);
        const first = ms.querySelector('[data-ms-menu] input[type="checkbox"]');
        if (first) first.focus();
      }
    });
    window.addEventListener('scroll', closeMultiSelects, true);
    window.addEventListener('resize', closeMultiSelects);
    document.addEventListener('click', function (event) {
      const telemetryTab = event.target.closest('[data-telemetry-tab]');
      if (telemetryTab) { setTelemetryView(telemetryTab.dataset.telemetryTab); return; }
      const toggle = event.target.closest('[data-toggle-password]');
      if (toggle) { const value = toggle.parentElement.querySelector('[data-auth-value]'); const visible = value.type === 'text'; value.type = visible ? 'password' : 'text'; toggle.title = visible ? '显示认证值' : '隐藏认证值'; toggle.setAttribute('aria-label', toggle.title); toggle.innerHTML = '<i data-lucide="' + (visible ? 'eye' : 'eye-off') + '"></i>'; renderIcons(toggle); return; }
      const caseToggle = event.target.closest('[data-toggle-case]');
      if (caseToggle) { const rule = caseToggle.closest('[data-rule]'); const enabled = rule.dataset.caseSensitive !== 'true'; rule.dataset.caseSensitive = String(enabled); caseToggle.classList.toggle('is-active', enabled); caseToggle.setAttribute('aria-pressed', String(enabled)); caseToggle.title = enabled ? '区分大小写：开' : '区分大小写：关'; caseToggle.setAttribute('aria-label', caseToggle.title); return; }
      const remove = event.target.closest('[data-delete-row]');
      if (remove) { remove.closest('tr[data-row]').remove(); updateSummary(); }
      const duplicateRow = event.target.closest('[data-duplicate-row]');
      if (duplicateRow) { const row = duplicateRow.closest('tr[data-row]'); const clone = row.cloneNode(true); row.after(clone); ensureDuplicateButtons(clone); renderMultiSelect(clone); renderIcons(clone); updateSummary(); return; }
      const addRule = event.target.closest('[data-add-rule]');
      if (addRule) { const menu = addRule.closest('[data-selector-rules]').querySelector('[data-rule-menu]'); menu.hidden = !menu.hidden; return; }
      const ruleOption = event.target.closest('[data-rule-type-option]');
      if (ruleOption) { const rules = ruleOption.closest('[data-selector-rules]'); const empty = rules.querySelector('[data-rule-empty]'); if (empty) { empty.insertAdjacentHTML('beforebegin', ruleRow(ruleOption.dataset.ruleTypeOption)); empty.hidden = true; } rules.querySelector('[data-rule-menu]').hidden = true; renderIcons(rules); return; }
      const deleteRule = event.target.closest('[data-rule-delete]');
      if (deleteRule) { const rules = deleteRule.closest('[data-selector-rules]'); deleteRule.closest('[data-rule]').remove(); const empty = rules.querySelector('[data-rule-empty]'); if (empty && !rules.querySelector('[data-rule]')) empty.hidden = false; renderIcons(rules); return; }
      if (!event.target.closest('[data-rule-menu]')) document.querySelectorAll('[data-rule-menu]').forEach(menu => menu.hidden = true);
      const deleteSelector = event.target.closest('[data-delete-selector]');
      if (deleteSelector) { deleteSelector.closest('[data-selector]').remove(); if (!selectorList.querySelector('[data-selector]')) selectorList.innerHTML = '<div class="selector-empty">暂无 AppSelector；未配置 selector 时保持原有 upstream 顺序。</div>'; updateSelectorSummary(); renderMultiSelect(document); }
      const duplicateSelector = event.target.closest('[data-duplicate-selector]');
      if (duplicateSelector) { const row = duplicateSelector.closest('[data-selector]'); const clone = row.cloneNode(true); row.after(clone); ensureDuplicateButtons(clone); renderIcons(clone); updateSelectorSummary(); renderMultiSelect(document); return; }
    });
    document.addEventListener('click', function (event) {
      const toggle = event.target.closest('[data-session-toggle]');
      if (toggle) { const card = toggle.closest('.session-card'); setSessionExpanded(card, toggle.getAttribute('aria-expanded') !== 'true'); return; }
      const full = event.target.closest('[data-payload-full]');
      if (full) { const preview = full.closest('.payload-preview'); const card = preview.closest('.session-card'); window.open('/sessions/' + encodeURIComponent(card.dataset.sessionId) + '/' + preview.querySelector('[data-session-payload]').dataset.sessionPayload + '?full=1', '_blank'); return; }
      const pretty = event.target.closest('[data-payload-pretty]');
      if (pretty) {
        const preview = pretty.closest('.payload-preview');
        const target = preview.querySelector('[data-session-payload]');
        const active = pretty.classList.contains('is-active');
        if (!active) {
          try { JSON.parse(target.dataset.raw || ''); } catch (_) { return; }
        }
        pretty.classList.toggle('is-active', !active);
        pretty.setAttribute('aria-pressed', String(!active));
        renderPayload(target);
        return;
      }
      const copy = event.target.closest('[data-payload-copy]');
      if (copy) {
        const preview = copy.closest('.payload-preview');
        const target = preview.querySelector('[data-session-payload]');
        const text = target.dataset.raw || target.textContent;
        copyText(text).then(() => {
          const original = copy.title;
          copy.title = '已复制';
          copy.setAttribute('aria-label', '已复制');
          copy.innerHTML = '<i data-lucide="check"></i>';
          renderIcons(copy);
          setTimeout(() => { copy.title = original; copy.setAttribute('aria-label', original); copy.innerHTML = '<i data-lucide="copy"></i>'; renderIcons(copy); }, 1200);
        }).catch(() => {});
        return;
      }
      const payloadToggle = event.target.closest('[data-payload-toggle]');
      if (payloadToggle) {
        const preview = payloadToggle.closest('.payload-preview');
        const collapsed = preview.classList.contains('is-collapsed');
        preview.classList.toggle('is-collapsed', !collapsed);
        payloadToggle.setAttribute('aria-expanded', String(collapsed));
        if (collapsed) {
          const target = preview.querySelector('[data-session-payload]');
          if (target && !target.dataset.raw) hydratePayload(target, preview.closest('.session-card').dataset.sessionId);
        }
        return;
      }
    });
    document.addEventListener('keydown', function (event) {
      const current = event.target.closest('[data-telemetry-tab]');
      if (!current || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
      const tabs = [...document.querySelectorAll('[data-telemetry-tab]')]; const index = tabs.indexOf(current); let next = index;
      if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length;
      if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
      if (event.key === 'Home') next = 0;
      if (event.key === 'End') next = tabs.length - 1;
      event.preventDefault(); setTelemetryView(tabs[next].dataset.telemetryTab, true);
    });
    document.body.addEventListener('htmx:afterSwap', function (event) { if (event.target === table) { ensureDuplicateButtons(table); renderMultiSelect(table); renderIcons(table); updateSummary(); } });
    document.body.addEventListener('htmx:sseMessage', function () { const logs = document.getElementById('log-stream'); logs.scrollTop = logs.scrollHeight; });
    fetch('/sessions').then(response => response.text()).then(reconcileSessionCards).catch(() => {});
    const sessionEvents = new EventSource('/sessions/stream');
    let pendingSessionHTML = null;
    let sessionReconcileTimer = null;
    sessionEvents.addEventListener('sessions', event => {
      pendingSessionHTML = event.data;
      if (sessionReconcileTimer) return;
      sessionReconcileTimer = setTimeout(() => {
        sessionReconcileTimer = null;
        if (pendingSessionHTML != null) { const html = pendingSessionHTML; pendingSessionHTML = null; reconcileSessionCards(html); }
      }, 80);
    });
    setInterval(refreshResponsePreviews, 800);
    setTheme(document.documentElement.dataset.theme || 'dark');
    updateSelectorSummary(); ensureDuplicateButtons(document); renderMultiSelect(document); renderIcons(document);
  </script>
</body>
</html>`))

var fragmentTemplate = template.Must(template.New("fragment").Parse(`{{range .}}<tr data-row draggable="true"><td class="priority"><span class="drag-handle" title="拖动排序"><i data-lucide="grip-vertical"></i></span></td><td><input class="field-input" data-name value="{{.Name}}" placeholder="名称" aria-label="上游名称"></td><td><input class="field-input" data-url value="{{.URL}}" aria-label="上游地址"></td><td><div class="auth"><select class="auth-select" data-auth-type aria-label="认证类型"><option value="none"{{if eq .AuthType "none"}} selected{{end}}>none</option><option value="basic"{{if eq .AuthType "basic"}} selected{{end}}>basic</option><option value="bearer"{{if eq .AuthType "bearer"}} selected{{end}}>bearer</option></select><span class="auth-value"><input class="field-input" data-auth-value type="password" value="{{.AuthValue}}" aria-label="认证值"></span><button class="icon-button" type="button" data-toggle-password title="显示认证值" aria-label="显示认证值"><i data-lucide="eye"></i></button></div></td><td class="app-selectors"><div class="multi-select" data-multi-select><input type="hidden" data-app-selectors value="{{.AppSelectorsText}}"><div class="ms-trigger" data-ms-trigger role="button" tabindex="0" aria-haspopup="listbox" aria-expanded="false" aria-label="兼容的 AppSelector"><span class="ms-chips" data-ms-chips></span><i data-lucide="chevron-down" class="ms-chevron"></i></div><div class="ms-menu" data-ms-menu hidden role="listbox" aria-multiselectable="true"></div></div></td><td class="row-actions"><button class="icon-button danger" type="button" data-delete-row title="删除上游" aria-label="删除上游"><i data-lucide="trash-2"></i></button></td></tr>{{else}}<tr><td colspan="6">没有配置上游</td></tr>{{end}}`))

func configViews(upstreams []Upstream) []configView {
	views := make([]configView, 0, len(upstreams))
	for i, upstream := range upstreams {
		view := configView{Index: i + 1, Name: upstream.Name, URL: upstream.URL, AppSelectorsText: strings.Join(upstream.AppSelectors, ", ")}
		if upstream.Authorization != nil {
			view.HasAuth = true
			view.AuthType = upstream.Authorization.Type
			view.AuthValue = upstream.Authorization.Value
		}
		views = append(views, view)
	}
	return views
}

func serveConfigPage(w http.ResponseWriter, _ *http.Request, selectors []AppSelector, debug bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := pageTemplate.Execute(w, pageView{Debug: debug, AppSelectors: selectors}); err != nil {
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func serveConfigFragment(w http.ResponseWriter, upstreams []Upstream) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := fragmentTemplate.Execute(w, configViews(upstreams)); err != nil {
		http.Error(w, "failed to render config", http.StatusInternalServerError)
	}
}
