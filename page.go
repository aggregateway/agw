package agw

import (
	"html/template"
	"net/http"
)

type configView struct {
	Index     int
	URL       string
	AuthType  string
	AuthValue string
	HasAuth   bool
}

type pageView struct {
	Debug bool
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AGW Control</title>
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>
  <script src="https://unpkg.com/lucide@0.468.0"></script>
  <style>
    :root { color-scheme: light; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    * { box-sizing: border-box; }
    .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
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
    .workspace-top { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 20px 14px; border-bottom: 1px solid #e5ece9; }
    .section-title { margin: 0; color: #213633; font-size: 15px; font-weight: 700; }
    .section-note { margin: 3px 0 0; color: #71827e; font-size: 12px; }
    .summary { display: flex; align-items: center; gap: 8px; color: #53716a; font-size: 12px; white-space: nowrap; }
    .summary-dot { width: 7px; height: 7px; background: #7fae33; border-radius: 50%; }
    #status { min-width: 72px; color: #637570; font-size: 12px; text-align: right; }
    #status.error { color: #a83e3e; }
    #status.success { color: #267d63; }
    .table-scroll { overflow-x: auto; }
    table { width: 100%; min-width: 840px; border-collapse: collapse; table-layout: fixed; }
    th, td { padding: 12px 14px; border-bottom: 1px solid #e7edeb; text-align: left; vertical-align: middle; }
    th { color: #778984; background: #f8faf9; font-size: 11px; font-weight: 700; letter-spacing: 0; text-transform: uppercase; }
    th.priority, td.priority { width: 48px; text-align: center; }
    th.endpoint { width: 38%; }
    th.authentication { width: 45%; }
    th.row-actions, td.row-actions { width: 56px; text-align: center; }
    tr:last-child td { border-bottom: 0; }
    tr[data-row]:hover td { background: #fbfdfc; }
    tr.dragging { opacity: .42; }
    .drag-handle { display: inline-grid; width: 28px; height: 32px; place-items: center; color: #91a29d; cursor: grab; user-select: none; }
    .drag-handle svg { width: 16px; height: 16px; }
    .field-input, .auth-select { width: 100%; height: 34px; min-width: 0; padding: 6px 8px; color: #203330; background: transparent; border: 1px solid transparent; border-radius: 4px; }
    .field-input { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 13px; }
    .field-input:hover, .auth-select:hover, .field-input:focus, .auth-select:focus { background: #fff; border-color: #b8c9c3; }
    .auth { display: grid; grid-template-columns: 112px minmax(0, 1fr) 34px; gap: 7px; align-items: center; }
    .auth-select { color: #30564d; font-size: 13px; font-weight: 600; }
    .auth-value { min-width: 0; }
    .auth-value .field-input { width: 100%; }
    .logs { margin-top: 20px; overflow: hidden; background: #13211f; border: 1px solid #29433e; border-radius: 8px; }
    .logs-top { display: flex; align-items: center; justify-content: space-between; padding: 12px 14px; color: #dce9e5; border-bottom: 1px solid #29433e; }
    .logs-title { display: flex; align-items: center; gap: 8px; margin: 0; font-size: 13px; font-weight: 700; }
    .live-dot { width: 7px; height: 7px; background: #b9e55a; border-radius: 50%; box-shadow: 0 0 0 3px rgba(185, 229, 90, .12); }
    .logs-meta { color: #9db5ae; font: 11px ui-monospace, SFMono-Regular, Menlo, monospace; }
    #log-stream { height: 250px; overflow: auto; padding: 14px; margin: 0; color: #c6d8d2; background: #13211f; white-space: pre-wrap; word-break: break-word; font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    #log-stream::selection { color: #13211f; background: #cbe86b; }
    @media (max-width: 760px) { .shell { width: min(100% - 24px, 1220px); margin-top: 12px; } .appbar { align-items: flex-start; flex-direction: column; padding: 16px; border-radius: 8px; } .appbar-actions { width: 100%; justify-content: flex-end; } .workspace { border-top: 1px solid #d7e0dc; border-radius: 8px; margin-top: 12px; } .workspace-top { padding: 15px; } .section-note { display: none; } .summary { display: none; } .logs { margin-top: 12px; } }
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
        <button class="icon-button" type="button" title="刷新配置" aria-label="刷新配置" hx-get="/config" hx-target="#config-table" hx-swap="innerHTML"><i data-lucide="refresh-cw"></i></button>
        <button class="icon-button" type="button" id="add-upstream" title="新增上游" aria-label="新增上游"><i data-lucide="plus"></i></button>
        <button class="icon-button save" type="button" id="save" title="保存配置" aria-label="保存配置"><i data-lucide="save"></i></button>
      </div>
    </header>
    <section class="workspace" aria-labelledby="routing-title">
      <div class="workspace-top">
        <div><h2 class="section-title" id="routing-title">Upstream routing</h2><p class="section-note">按显示顺序重试，拖动行可调整优先级</p></div>
        <div class="summary"><span class="summary-dot"></span><span id="upstream-count">加载中</span></div>
      </div>
      <div class="table-scroll">
        <table>
          <thead><tr><th class="priority" scope="col">优先级</th><th class="endpoint" scope="col">Upstream endpoint</th><th class="authentication" scope="col">Authentication</th><th class="row-actions" scope="col"><span class="sr-only">操作</span></th></tr></thead>
          <tbody id="config-table" hx-get="/config" hx-trigger="load" hx-swap="innerHTML"><tr><td colspan="4">正在加载上游配置...</td></tr></tbody>
        </table>
      </div>
    </section>
    <section class="logs" aria-labelledby="logs-title">
      <div class="logs-top"><h2 class="logs-title" id="logs-title"><span class="live-dot"></span>Live request feed</h2><span class="logs-meta">SSE connected</span></div>
      <pre id="log-stream" hx-ext="sse" sse-connect="/logs" sse-swap="message" hx-swap="beforeend"></pre>
    </section>
  </main>
  <script>
    const table = document.getElementById('config-table');
    const status = document.getElementById('status');
    const saveButton = document.getElementById('save');
    let dragged;
    function renderIcons(scope) { if (window.lucide) window.lucide.createIcons({root: scope || document, attrs: {'stroke-width': 1.8}}); }
    function updateSummary() { document.getElementById('upstream-count').textContent = table.querySelectorAll('tr[data-row]').length + ' upstreams'; }
    function newRow() { return '<tr data-row draggable="true"><td class="priority"><span class="drag-handle" title="拖动排序"><i data-lucide="grip-vertical"></i></span></td><td><input class="field-input" data-url value="https://example.com/v1" aria-label="上游地址"></td><td><div class="auth"><select class="auth-select" data-auth-type aria-label="认证类型"><option value="none" selected>none</option><option value="basic">basic</option><option value="bearer">bearer</option></select><span class="auth-value"><input class="field-input" data-auth-value type="password" value="" aria-label="认证值"></span><button class="icon-button" type="button" data-toggle-password title="显示认证值" aria-label="显示认证值"><i data-lucide="eye"></i></button></div></td><td class="row-actions"><button class="icon-button danger" type="button" data-delete-row title="删除上游" aria-label="删除上游"><i data-lucide="trash-2"></i></button></td></tr>'; }
    document.addEventListener('dragstart', function (event) { const row = event.target.closest('tr[data-row]'); if (!row) return; dragged = row; row.classList.add('dragging'); });
    document.addEventListener('dragend', function () { if (dragged) dragged.classList.remove('dragging'); dragged = null; });
    document.addEventListener('dragover', function (event) { const row = event.target.closest('tr[data-row]'); if (!dragged || !row || row === dragged) return; event.preventDefault(); const box = row.getBoundingClientRect(); row.parentNode.insertBefore(dragged, event.clientY < box.top + box.height / 2 ? row : row.nextSibling); });
    saveButton.addEventListener('click', async function () {
      const upstreams = [...table.querySelectorAll('tr[data-row]')].map(row => ({url: row.querySelector('[data-url]').value.trim(), authorization: {type: row.querySelector('[data-auth-type]').value, value: row.querySelector('[data-auth-value]').value}}));
      status.className = ''; status.textContent = '保存中'; saveButton.disabled = true;
      try {
        const response = await fetch('/config', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({debug: document.getElementById('debug-toggle').checked, upstreams})});
        if (!response.ok) throw new Error(await response.text());
        status.className = 'success'; status.textContent = '已保存'; htmx.trigger(table, 'load');
      } catch (error) { status.className = 'error'; status.textContent = error.message || '保存失败'; }
      finally { saveButton.disabled = false; }
    });
    document.getElementById('add-upstream').addEventListener('click', function () { table.insertAdjacentHTML('beforeend', newRow()); renderIcons(table); updateSummary(); });
    document.addEventListener('click', function (event) {
      const toggle = event.target.closest('[data-toggle-password]');
      if (toggle) { const value = toggle.parentElement.querySelector('[data-auth-value]'); const visible = value.type === 'text'; value.type = visible ? 'password' : 'text'; toggle.title = visible ? '显示认证值' : '隐藏认证值'; toggle.setAttribute('aria-label', toggle.title); toggle.innerHTML = '<i data-lucide="' + (visible ? 'eye' : 'eye-off') + '"></i>'; renderIcons(toggle); return; }
      const remove = event.target.closest('[data-delete-row]');
      if (remove) { remove.closest('tr[data-row]').remove(); updateSummary(); }
    });
    document.body.addEventListener('htmx:afterSwap', function (event) { if (event.target === table) { renderIcons(table); updateSummary(); } });
    document.body.addEventListener('htmx:sseMessage', function () { const logs = document.getElementById('log-stream'); logs.scrollTop = logs.scrollHeight; });
    renderIcons(document);
  </script>
</body>
</html>`))

var fragmentTemplate = template.Must(template.New("fragment").Parse(`{{range .}}<tr data-row draggable="true"><td class="priority"><span class="drag-handle" title="拖动排序"><i data-lucide="grip-vertical"></i></span></td><td><input class="field-input" data-url value="{{.URL}}" aria-label="上游地址"></td><td><div class="auth"><select class="auth-select" data-auth-type aria-label="认证类型"><option value="none"{{if eq .AuthType "none"}} selected{{end}}>none</option><option value="basic"{{if eq .AuthType "basic"}} selected{{end}}>basic</option><option value="bearer"{{if eq .AuthType "bearer"}} selected{{end}}>bearer</option></select><span class="auth-value"><input class="field-input" data-auth-value type="password" value="{{.AuthValue}}" aria-label="认证值"></span><button class="icon-button" type="button" data-toggle-password title="显示认证值" aria-label="显示认证值"><i data-lucide="eye"></i></button></div></td><td class="row-actions"><button class="icon-button danger" type="button" data-delete-row title="删除上游" aria-label="删除上游"><i data-lucide="trash-2"></i></button></td></tr>{{else}}<tr><td colspan="4">没有配置上游</td></tr>{{end}}`))

func configViews(upstreams []Upstream) []configView {
	views := make([]configView, 0, len(upstreams))
	for i, upstream := range upstreams {
		view := configView{Index: i + 1, URL: upstream.URL}
		if upstream.Authorization != nil {
			view.HasAuth = true
			view.AuthType = upstream.Authorization.Type
			view.AuthValue = upstream.Authorization.Value
		}
		views = append(views, view)
	}
	return views
}

func serveConfigPage(w http.ResponseWriter, _ *http.Request, _ []Upstream, debug bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, pageView{Debug: debug}); err != nil {
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func serveConfigFragment(w http.ResponseWriter, upstreams []Upstream) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := fragmentTemplate.Execute(w, configViews(upstreams)); err != nil {
		http.Error(w, "failed to render config", http.StatusInternalServerError)
	}
}
