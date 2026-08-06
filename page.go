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

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AGW Configuration</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>
  <style>
    :root { color-scheme: light; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; color: #17202a; background: #f4f6f8; }
    main { max-width: 1080px; margin: 0 auto; padding: 40px 24px; }
    header { display: flex; align-items: end; justify-content: space-between; gap: 20px; margin-bottom: 24px; }
    h1 { margin: 0; font-size: 28px; }
    p { color: #64717e; }
    button { border: 1px solid #cbd3da; border-radius: 6px; background: white; padding: 9px 14px; cursor: pointer; }
    button:hover { background: #eef2f5; }
    .primary { color: white; background: #1769aa; border-color: #1769aa; }
    .primary:hover { background: #12598f; }
    .table-wrap { overflow-x: auto; background: white; border: 1px solid #dce2e7; border-radius: 8px; }
    table { width: 100%; border-collapse: collapse; min-width: 680px; }
    th, td { padding: 15px 16px; border-bottom: 1px solid #e8ecef; text-align: left; vertical-align: top; }
    th { color: #64717e; font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
    tr:last-child td { border-bottom: 0; }
    code { word-break: break-all; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; }
    .order { color: #64717e; width: 56px; }
    .auth { display: grid; gap: 6px; }
    .auth-value { display: flex; gap: 6px; align-items: center; }
    .auth-value input { flex: 1; min-width: 0; }
    .toggle-password { flex: 0 0 auto; white-space: nowrap; padding: 7px 10px; }
    .muted { color: #8a96a1; }
    .drag { cursor: grab; color: #8a96a1; user-select: none; }
    tr.dragging { opacity: .45; }
    input, select { width: 100%; box-sizing: border-box; border: 1px solid transparent; border-radius: 4px; padding: 7px; font: inherit; background: transparent; }
    input:hover, input:focus { border-color: #cbd3da; background: white; outline: none; }
    select:hover, select:focus { border-color: #cbd3da; background: white; outline: none; }
    .actions { display: flex; gap: 8px; align-items: center; }
    #status { color: #64717e; font-size: 14px; }
    .logs { margin-top: 24px; }
    .logs h2 { font-size: 18px; margin: 0 0 10px; }
    #log-stream { height: 220px; overflow: auto; padding: 14px; margin: 0; color: #dbe7f2; background: #17202a; border-radius: 8px; white-space: pre-wrap; word-break: break-word; font: 12px/1.55 ui-monospace, SFMono-Regular, Menlo, monospace; }
    @media (max-width: 700px) { main { padding: 24px 14px; } header { align-items: start; flex-direction: column; } }
  </style>
</head>
<body>
  <main>
    <header>
      <div><h1>AGW Configuration</h1><p>按顺序尝试的上游与认证配置</p></div>
      <div class="actions"><span id="status"></span><button hx-get="/config" hx-target="#config-table" hx-swap="innerHTML">刷新</button><button class="primary" type="button" id="save">保存</button></div>
    </header>
    <section class="table-wrap">
      <table>
        <thead><tr><th>顺序</th><th>上游地址</th><th>认证</th></tr></thead>
        <tbody id="config-table" hx-get="/config" hx-trigger="load" hx-swap="innerHTML"><tr><td colspan="3">加载中...</td></tr></tbody>
      </table>
    </section>
    <section class="logs">
      <h2>实时日志</h2>
      <pre id="log-stream" hx-ext="sse" sse-connect="/logs" sse-swap="message" hx-swap="beforeend">正在连接日志流...</pre>
    </section>
  </main>
  <script>
    const table = document.getElementById('config-table');
    let dragged;
    document.addEventListener('dragstart', function (event) {
      const row = event.target.closest('tr[data-row]');
      if (!row) return;
      dragged = row;
      row.classList.add('dragging');
    });
    document.addEventListener('dragend', function () {
      if (dragged) dragged.classList.remove('dragging');
      dragged = null;
    });
    document.addEventListener('dragover', function (event) {
      const row = event.target.closest('tr[data-row]');
      if (!dragged || !row || row === dragged) return;
      event.preventDefault();
      const box = row.getBoundingClientRect();
      row.parentNode.insertBefore(dragged, event.clientY < box.top + box.height / 2 ? row : row.nextSibling);
    });
    document.getElementById('save').addEventListener('click', async function () {
      const rows = [...table.querySelectorAll('tr[data-row]')];
      const config = rows.map(row => ({
        url: row.querySelector('[data-url]').value.trim(),
        authorization: {
          type: row.querySelector('[data-auth-type]').value.trim(),
          value: row.querySelector('[data-auth-value]').value
        }
      }));
      const status = document.getElementById('status');
      status.textContent = '保存中...';
      const response = await fetch('/config', { method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(config) });
      if (!response.ok) { status.textContent = await response.text(); return; }
      status.textContent = '已保存';
      htmx.trigger(table, 'load');
    });
    document.addEventListener('click', function (event) {
      const button = event.target.closest('[data-toggle-password]');
      if (!button) return;
      const value = button.parentElement.querySelector('[data-auth-value]');
      const visible = value.type === 'text';
      value.type = visible ? 'password' : 'text';
      button.textContent = visible ? '显示' : '隐藏';
    });
    document.body.addEventListener('htmx:sseMessage', function () {
      const logs = document.getElementById('log-stream');
      logs.scrollTop = logs.scrollHeight;
    });
  </script>
</body>
</html>`))

var fragmentTemplate = template.Must(template.New("fragment").Parse(`{{range .}}<tr data-row draggable="true"><td class="order drag" title="拖动排序">↕</td><td><input data-url value="{{.URL}}"></td><td><div class="auth"><select data-auth-type><option value="none"{{if eq .AuthType "none"}} selected{{end}}>none</option><option value="basic"{{if eq .AuthType "basic"}} selected{{end}}>basic</option><option value="bearer"{{if eq .AuthType "bearer"}} selected{{end}}>bearer</option></select><span class="auth-value"><input data-auth-value type="password" value="{{.AuthValue}}"><button class="toggle-password" type="button" data-toggle-password>显示</button></span></div></td></tr>{{else}}<tr><td colspan="3">没有配置上游</td></tr>{{end}}`))

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

func serveConfigPage(w http.ResponseWriter, _ *http.Request, upstreams []Upstream) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, nil); err != nil {
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func serveConfigFragment(w http.ResponseWriter, upstreams []Upstream) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := fragmentTemplate.Execute(w, configViews(upstreams)); err != nil {
		http.Error(w, "failed to render config", http.StatusInternalServerError)
	}
}
