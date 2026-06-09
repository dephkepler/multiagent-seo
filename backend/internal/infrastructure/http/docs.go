package http

import (
	"encoding/json"
	"net/http"
	"sync"

	"multiagent-seo/internal/oapigen"
)

var (
	specOnce  sync.Once
	specBytes []byte
	specErr   error
)

func openAPISpecJSON() ([]byte, error) {
	specOnce.Do(func() {
		swagger, err := oapigen.GetSwagger()
		if err != nil {
			specErr = err
			return
		}
		specBytes, specErr = json.Marshal(swagger)
	})
	return specBytes, specErr
}

func handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	b, err := openAPISpecJSON()
	if err != nil {
		http.Error(w, "openapi spec unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(b)
}

func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func handleLandingPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write([]byte(landingPageHTML))
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>ContentFlow API — Swagger UI</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.json",
      dom_id: "#swagger-ui",
      deepLinking: true,
      tryItOutEnabled: true,
      // Keep the Authorize token in localStorage so a page reload doesn't drop it.
      persistAuthorization: true,
    });
  </script>
</body>
</html>`

const landingPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Contentflow</title>
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0; min-height: 100vh; padding: 32px 24px;
      font: 14px/1.4 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #f5f5f5; color: #1a1a1a;
    }
    .wrap { max-width: 1100px; margin: 0 auto; }
    .card { background: white; padding: 24px; border-radius: 4px; margin-bottom: 16px; }
    h1 { margin: 0; font-size: 18px; font-weight: 600; }
    .top-bar {
      display: flex; gap: 12px; align-items: center; justify-content: space-between;
      padding: 8px 12px; margin-bottom: 16px;
      background: #f0f5ff; border: 1px solid #adc6ff; border-radius: 4px;
      font-size: 12px; color: #595959;
    }
    .top-bar .links a { margin-left: 12px; }
    .add-site {
      display: grid; grid-template-columns: 1fr 1.5fr 1fr 1.5fr auto; gap: 12px; align-items: end;
    }
    .add-site input { padding: 6px 8px; border: 1px solid #d9d9d9; border-radius: 3px; font-size: 13px; }
    .add-site button {
      padding: 7px 16px; background: #1677ff; color: white; border: none;
      border-radius: 3px; font-size: 13px; cursor: pointer;
    }
    .add-site button:hover { background: #4096ff; }
    .add-site button:disabled { background: #d9d9d9; cursor: wait; }
    form .row {
      display: grid;
      grid-template-columns: 1.5fr 1fr 1fr 1fr 1.2fr;
      gap: 16px;
      align-items: start;
    }
    label { display: block; font-size: 12px; color: #595959; margin-bottom: 4px; }
    input, select {
      width: 100%; padding: 6px 8px; border: 1px solid #d9d9d9;
      border-radius: 3px; font-size: 13px; background: white;
    }
    input:focus, select:focus { outline: none; border-color: #4096ff; }
    .col-words input { margin-bottom: 6px; }
    .toggle { display: flex; gap: 12px; padding: 6px 10px; border: 1px solid #d9d9d9; border-radius: 3px; }
    .toggle label {
      display: flex; align-items: center; gap: 6px;
      margin: 0; font-size: 13px; color: #1a1a1a; cursor: pointer;
    }
    .toggle input { width: auto; }
    .create-btn {
      display: block; margin: 24px auto 0; padding: 10px 80px;
      background: #95de64; color: #1a1a1a; border: none; border-radius: 3px;
      font-size: 14px; font-weight: 500; cursor: pointer;
    }
    .create-btn:hover { background: #82d44b; }
    .create-btn:disabled { background: #d9d9d9; cursor: wait; }
    .msg { margin-top: 12px; padding: 8px 12px; border-radius: 3px; font-size: 13px; display: none; }
    .msg.ok { background: #f6ffed; border: 1px solid #b7eb8f; color: #389e0d; }
    .msg.err { background: #fff2f0; border: 1px solid #ffccc7; color: #cf1322; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { padding: 8px 10px; text-align: left; border-bottom: 1px solid #f0f0f0; vertical-align: top; }
    th { font-weight: 600; color: #595959; font-size: 12px; }
    .badge { display: inline-block; padding: 2px 8px; border-radius: 10px; font-size: 11px; font-weight: 500; }
    .badge.generating { background: #fff7e6; color: #d46b08; }
    .badge.draft { background: #e6f7ff; color: #0958d9; }
    .badge.published { background: #f6ffed; color: #389e0d; }
    .badge.failed { background: #fff2f0; color: #cf1322; }
    .badge.unknown { background: #f5f5f5; color: #595959; }
    a { color: #1677ff; text-decoration: none; }
    a:hover { text-decoration: underline; }
    .hdr { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
    .refresh { background: white; border: 1px solid #d9d9d9; padding: 4px 12px; border-radius: 3px; cursor: pointer; font-size: 12px; }
    .refresh:hover { background: #f5f5f5; }
    .refresh:disabled { opacity: 0.6; cursor: wait; }
    .refresh-info { font-size: 12px; color: #8c8c8c; margin-left: 8px; }
    .refresh-info.pulse { color: #1677ff; }
    @keyframes flash { 0% { background: #fffbe6; } 100% { background: transparent; } }
    tr.flash > td { animation: flash 1.2s ease-out; }
    .elapsed-live { color: #d46b08; font-variant-numeric: tabular-nums; }
    .elapsed-done { color: #595959; font-variant-numeric: tabular-nums; }
    footer { color: #8c8c8c; font-size: 12px; text-align: center; padding: 16px 0; }
    footer a { color: #8c8c8c; margin: 0 8px; }
    .muted { color: #8c8c8c; padding: 20px; text-align: center; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="top-bar">
      <span id="auth-status">signing in…</span>
      <span class="links">
        <a href="/docs">Swagger ↗</a>
      </span>
    </div>

    <div class="card">
      <h1 style="margin-bottom:12px">Add WordPress Site</h1>
      <form id="add-site-form" class="add-site">
        <div>
          <label>Alias</label>
          <input name="alias" placeholder="playpulse" required />
        </div>
        <div>
          <label>URL</label>
          <input name="url" type="url" placeholder="https://example.com" required />
        </div>
        <div>
          <label>Username</label>
          <input name="username" placeholder="admin" required />
        </div>
        <div>
          <label>App password</label>
          <input name="appPassword" type="password" placeholder="xxxx xxxx xxxx xxxx" required />
        </div>
        <button type="submit">Add</button>
      </form>
      <div id="add-site-msg" class="msg"></div>
    </div>

    <div class="card">
      <form id="gen-form">
        <div class="row">
          <div>
            <label>Keyword</label>
            <input name="keyword" required />
            <label style="margin-top:8px">Website</label>
            <select name="site_id" id="site-select">
              <option value="">— loading…</option>
            </select>
          </div>
          <div class="col-words">
            <label>Words Quantity</label>
            <input name="min_words" type="number" placeholder="Min" />
            <input name="max_words" type="number" placeholder="Max" />
          </div>
          <div>
            <label>Tokens Quantity</label>
            <input name="max_tokens" type="number" value="1000" />
          </div>
          <div>
            <label>Language</label>
            <select name="language">
              <option value="en">English</option>
              <option value="ru">Russian</option>
              <option value="uk">Ukrainian</option>
            </select>
            <label style="margin-top:8px">Provider</label>
            <select name="provider">
              <option value="groq">Groq (llama-3.3-70b)</option>
              <option value="claude">Claude (haiku-4-5)</option>
            </select>
            <label style="margin-top:8px">Cycles</label>
            <select name="cycles">
              <option value="1">Fast (1 cycle)</option>
              <option value="2">Normal (2 cycles)</option>
              <option value="3" selected>Quality (3 cycles)</option>
            </select>
          </div>
          <div>
            <label>Autopublishing</label>
            <div class="toggle">
              <label><input type="radio" name="autopublish" value="true"/> Yes</label>
              <label><input type="radio" name="autopublish" value="false" checked/> No</label>
            </div>
            <label style="margin-top:8px">Images</label>
            <div class="toggle">
              <label><input type="radio" name="images" value="true" checked/> Yes</label>
              <label><input type="radio" name="images" value="false"/> No</label>
            </div>
            <label style="margin-top:8px">AI Threshold</label>
            <input name="ai_threshold" type="number" step="0.05" min="0" max="1" value="0.8" />
          </div>
        </div>
        <button type="submit" class="create-btn">Create</button>
        <div id="msg" class="msg"></div>
      </form>
    </div>

    <div class="card">
      <div class="hdr">
        <h1>Articles</h1>
        <div>
          <button id="refresh" class="refresh" type="button">Refresh</button>
          <span id="refresh-info" class="refresh-info"></span>
        </div>
      </div>
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Keyword</th>
            <th>Site</th>
            <th>Status</th>
            <th>Created</th>
            <th>Elapsed</th>
            <th>WordPress</th>
          </tr>
        </thead>
        <tbody id="rows">
          <tr><td colspan="7" class="muted">Sign in (paste a token above) to load articles.</td></tr>
        </tbody>
      </table>
    </div>

    <footer>
      <a href="/docs">Swagger UI</a> · <a href="/openapi.json">openapi.json</a>
    </footer>
  </div>

  <script>
    var TOKEN_KEY = "mas_token";
    var AUTO_EMAIL = "verify@local.test";
    var AUTO_PASSWORD = "verify123";

    var authStatus = document.getElementById("auth-status");
    var siteSelect = document.getElementById("site-select");
    var form = document.getElementById("gen-form");
    var msg = document.getElementById("msg");
    var rows = document.getElementById("rows");
    var btn = form.querySelector("button.create-btn");
    var refreshBtn = document.getElementById("refresh");
    var refreshInfo = document.getElementById("refresh-info");
    var addSiteForm = document.getElementById("add-site-form");
    var addSiteMsg = document.getElementById("add-site-msg");
    var addSiteBtn = addSiteForm.querySelector("button");

    var sitesById = {};
    var lastFetchAt = null;
    var isFetching = false;
    var seenStatus = {};
    var latestArticles = [];

    function getToken() { return localStorage.getItem(TOKEN_KEY) || ""; }
    function setToken(t) {
      if (t) localStorage.setItem(TOKEN_KEY, t);
      else localStorage.removeItem(TOKEN_KEY);
    }
    function authHeaders() {
      var t = getToken();
      return t ? { "Authorization": "Bearer " + t } : {};
    }

    // Silent auto-login: dashboard does POST /auth/login with the shared service
    // account when no valid token is on file. Users never see a login prompt.
    function autoLogin() {
      return fetch("/auth/login", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify({email: AUTO_EMAIL, password: AUTO_PASSWORD})
      }).then(function(r) {
        if (!r.ok) throw new Error("auto-login HTTP " + r.status);
        return r.json();
      }).then(function(j) {
        setToken(j.token);
        authStatus.textContent = "signed in";
      });
    }

    function ensureAuth() {
      if (getToken()) {
        authStatus.textContent = "signed in";
        return Promise.resolve();
      }
      authStatus.textContent = "signing in…";
      return autoLogin();
    }

    function showMsg(text, ok) {
      msg.textContent = text;
      msg.className = "msg " + (ok ? "ok" : "err");
      msg.style.display = "block";
    }
    function num(v) { var n = parseInt(v, 10); return isNaN(n) ? 0 : n; }
    function esc(s) {
      return String(s == null ? "" : s).replace(/[&<>"']/g, function(c) {
        return ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[c];
      });
    }
    function fmtDate(s) {
      if (!s) return "";
      var d = new Date(s);
      return isNaN(d.getTime()) ? s : d.toLocaleString();
    }
    function fmtElapsed(secs) {
      if (secs < 0) return "0s";
      if (secs < 60) return secs + "s";
      var m = Math.floor(secs / 60), s = secs % 60;
      return m + "m " + s + "s";
    }
    function elapsedFor(a) {
      var start = new Date(a.created_at).getTime();
      if (isNaN(start)) return { secs: 0, live: false };
      var end = (a.status === "generating") ? Date.now() : new Date(a.updated_at || a.created_at).getTime();
      return { secs: Math.max(0, Math.floor((end - start) / 1000)), live: a.status === "generating" };
    }
    function badge(s) {
      var known = {generating:1, draft:1, published:1, failed:1};
      var cls = known[s] ? s : "unknown";
      return '<span class="badge ' + cls + '">' + esc(s || "unknown") + '</span>';
    }
    function siteAlias(id) {
      var s = sitesById[id];
      return s ? s.alias : (id ? id.slice(0, 8) : "—");
    }
    function viewURL(a) {
      var s = sitesById[a.site_id];
      if (!s || !a.wp_post_id) return null;
      return s.url.replace(/\/+$/, "") + "/?p=" + a.wp_post_id;
    }
    function wpLinks(a) {
      var parts = [];
      if (a.wp_edit_url) parts.push('<a href="' + esc(a.wp_edit_url) + '" target="_blank">edit</a>');
      var view = viewURL(a);
      if (view && a.status === "published") parts.push('<a href="' + esc(view) + '" target="_blank">view</a>');
      if (a.status === "draft" && a.wp_edit_url) {
        parts.push('<a href="#" data-publish="' + a.id + '">publish</a>');
      }
      return parts.join(" · ") || '<span style="color:#bfbfbf">—</span>';
    }

    function authFetch(url, opts) {
      opts = opts || {};
      opts.headers = Object.assign({}, opts.headers || {}, authHeaders());
      return fetch(url, opts);
    }

    function publishArticle(id) {
      if (!confirm("Publish article #" + id + " to WordPress?")) return;
      authFetch("/articles/" + id + "/publish", { method: "POST" })
        .then(function(r) { return r.text().then(function(t) { return [r.ok, r.status, t]; }); })
        .then(function(tpl) {
          if (tpl[0]) {
            showMsg("Published #" + id, true);
            loadArticles();
          } else {
            showMsg("Publish failed (" + tpl[1] + "): " + tpl[2], false);
          }
        }).catch(function(e) {
          showMsg("Network error: " + e, false);
        });
    }

    rows.addEventListener("click", function(e) {
      var t = e.target;
      if (t.tagName === "A" && t.dataset.publish) {
        e.preventDefault();
        publishArticle(parseInt(t.dataset.publish, 10));
      }
    });

    addSiteForm.addEventListener("submit", function(e) {
      e.preventDefault();
      var data = new FormData(addSiteForm);
      var body = {
        alias: (data.get("alias") || "").trim(),
        url: (data.get("url") || "").trim(),
        username: (data.get("username") || "").trim(),
        appPassword: data.get("appPassword") || ""
      };
      addSiteBtn.disabled = true;
      addSiteMsg.style.display = "none";
      authFetch("/wordpress-sites", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify(body)
      }).then(function(r) {
        return r.text().then(function(t) {
          var j; try { j = JSON.parse(t); } catch (_) { j = { message: t }; }
          return [r.ok, r.status, j];
        });
      }).then(function(tpl) {
        if (tpl[0]) {
          addSiteMsg.className = "msg ok";
          addSiteMsg.textContent = "Added site " + tpl[2].alias;
          addSiteMsg.style.display = "block";
          addSiteForm.reset();
          loadSites();
        } else {
          addSiteMsg.className = "msg err";
          addSiteMsg.textContent = "Error (" + tpl[1] + "): " + (tpl[2].title || tpl[2].detail || tpl[2].message || "unknown");
          addSiteMsg.style.display = "block";
        }
      }).catch(function(err) {
        addSiteMsg.className = "msg err";
        addSiteMsg.textContent = "Network error: " + err;
        addSiteMsg.style.display = "block";
      }).finally(function() {
        addSiteBtn.disabled = false;
      });
    });

    form.addEventListener("submit", function(e) {
      e.preventDefault();
      var data = new FormData(form);
      var provider = data.get("provider") || "groq";
      var modelByProvider = {
        groq: "llama-3.3-70b-versatile",
        claude: "claude-haiku-4-5-20251001"
      };
      var threshold = parseFloat(data.get("ai_threshold"));
      var body = {
        keyword: data.get("keyword") || "",
        site_id: data.get("site_id") || "",
        min_words: num(data.get("min_words")),
        max_words: num(data.get("max_words")),
        max_tokens: num(data.get("max_tokens")),
        language: data.get("language") || "",
        provider: provider,
        model: modelByProvider[provider] || "",
        max_cycles: num(data.get("cycles")),
        ai_threshold: isNaN(threshold) ? 0 : threshold,
        auto_publish: data.get("autopublish") === "true",
        include_images: data.get("images") === "true",
        // Pexels attribution keeps creeping back into published posts; the
        // dashboard always opts out so we don't have to remember.
        include_image_attribution: false
      };
      btn.disabled = true; btn.textContent = "Creating…";
      authFetch("/generate", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify(body)
      }).then(function(r) {
        return r.text().then(function(t) {
          var j; try { j = JSON.parse(t); } catch (_) { j = { message: t }; }
          return [r.ok, r.status, j];
        });
      }).then(function(tpl) {
        if (tpl[0]) {
          showMsg("Queued: id=" + tpl[2].id + " (" + tpl[2].status + ")", true);
          loadArticles();
        } else {
          showMsg("Error (" + tpl[1] + "): " + (tpl[2].title || tpl[2].detail || tpl[2].message || "unknown"), false);
        }
      }).catch(function(e) {
        showMsg("Network error: " + e, false);
      }).finally(function() {
        btn.disabled = false; btn.textContent = "Create";
      });
    });

    function loadSites() {
      if (!getToken()) {
        siteSelect.innerHTML = '<option value="">— sign in first —</option>';
        return Promise.resolve();
      }
      return authFetch("/wordpress-sites").then(function(r) {
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      }).then(function(list) {
        sitesById = {};
        var enabled = (list || []).filter(function(s) { return s.enabled; });
        enabled.forEach(function(s) { sitesById[s.id] = s; });
        (list || []).forEach(function(s) { sitesById[s.id] = s; });
        if (enabled.length === 0) {
          siteSelect.innerHTML = '<option value="">— no sites configured —</option>';
        } else {
          siteSelect.innerHTML = enabled.map(function(s) {
            return '<option value="' + esc(s.id) + '">' + esc(s.alias) + '</option>';
          }).join("");
        }
      }).catch(function(e) {
        siteSelect.innerHTML = '<option value="">— ' + esc(String(e.message || e)) + ' —</option>';
      });
    }

    function renderTable() {
      var list = latestArticles;
      if (list.length === 0) {
        rows.innerHTML = '<tr><td colspan="7" class="muted">No articles yet</td></tr>';
        return;
      }
      rows.innerHTML = list.slice(0, 30).map(function(a) {
        var changed = seenStatus[a.id] && seenStatus[a.id] !== a.status;
        var el = elapsedFor(a);
        var elCls = el.live ? "elapsed-live" : "elapsed-done";
        return "<tr" + (changed ? ' class="flash"' : '') + ">" +
          "<td>" + a.id + "</td>" +
          "<td>" + esc(a.keyword) + "</td>" +
          "<td>" + esc(siteAlias(a.site_id)) + "</td>" +
          "<td>" + badge(a.status) + "</td>" +
          "<td>" + fmtDate(a.created_at) + "</td>" +
          '<td class="' + elCls + '">' + fmtElapsed(el.secs) + (el.live ? " ⏱" : "") + "</td>" +
          "<td>" + wpLinks(a) + "</td>" +
        "</tr>";
      }).join("");
      list.forEach(function(a) { seenStatus[a.id] = a.status; });
    }

    function updateRefreshInfo() {
      if (isFetching) {
        refreshInfo.textContent = "refreshing…";
        refreshInfo.classList.add("pulse");
        return;
      }
      refreshInfo.classList.remove("pulse");
      if (!lastFetchAt) { refreshInfo.textContent = ""; return; }
      var secs = Math.floor((Date.now() - lastFetchAt) / 1000);
      refreshInfo.textContent = (secs === 0 ? "just refreshed" : "refreshed " + secs + "s ago");
    }

    function loadArticles() {
      if (!getToken()) {
        rows.innerHTML = '<tr><td colspan="7" class="muted">Sign in (paste a token above) to load articles.</td></tr>';
        return;
      }
      isFetching = true;
      refreshBtn.disabled = true;
      updateRefreshInfo();
      authFetch("/articles").then(function(r) {
        if (r.status === 401) { throw new Error("unauthorized — token expired or invalid"); }
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      }).then(function(list) {
        if (!Array.isArray(list)) list = [];
        list.sort(function(a, b) { return (b.id || 0) - (a.id || 0); });
        latestArticles = list;
        lastFetchAt = Date.now();
        renderTable();
        reschedulePolling();
      }).catch(function(e) {
        rows.innerHTML = '<tr><td colspan="7" style="color:#cf1322" class="muted">Load failed: ' + esc(String(e.message || e)) + '</td></tr>';
      }).finally(function() {
        isFetching = false;
        refreshBtn.disabled = false;
        updateRefreshInfo();
      });
    }

    function tick() {
      updateRefreshInfo();
      var hasLive = latestArticles.some(function(a) { return a.status === "generating"; });
      if (hasLive) renderTable();
    }

    var ACTIVE_MS = 5000, IDLE_MS = 30000;
    var pollTimer = null, currentInterval = 0;
    function reschedulePolling() {
      var hasLive = latestArticles.some(function(a) { return a.status === "generating"; });
      var target = hasLive ? ACTIVE_MS : IDLE_MS;
      if (target === currentInterval) return;
      if (pollTimer) clearInterval(pollTimer);
      pollTimer = setInterval(loadArticles, target);
      currentInterval = target;
    }

    refreshBtn.addEventListener("click", loadArticles);

    function loadArticlesWithRefresh() {
      if (!getToken()) {
        rows.innerHTML = '<tr><td colspan="7" class="muted">No token — refresh the page.</td></tr>';
        return;
      }
      isFetching = true;
      refreshBtn.disabled = true;
      updateRefreshInfo();
      authFetch("/articles").then(function(r) {
        if (r.status === 401) {
          // Token expired — re-auth silently and retry once.
          setToken("");
          return autoLogin().then(function() { return authFetch("/articles"); });
        }
        return r;
      }).then(function(r) {
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      }).then(function(list) {
        if (!Array.isArray(list)) list = [];
        list.sort(function(a, b) { return (b.id || 0) - (a.id || 0); });
        latestArticles = list;
        lastFetchAt = Date.now();
        renderTable();
        reschedulePolling();
      }).catch(function(e) {
        rows.innerHTML = '<tr><td colspan="7" style="color:#cf1322" class="muted">Load failed: ' + esc(String(e.message || e)) + '</td></tr>';
      }).finally(function() {
        isFetching = false;
        refreshBtn.disabled = false;
        updateRefreshInfo();
      });
    }

    // Override the simpler loadArticles with the retry-on-401 version.
    loadArticles = loadArticlesWithRefresh;

    ensureAuth().then(function() {
      loadSites();
      loadArticles();
      setInterval(tick, 1000);
    }).catch(function(e) {
      authStatus.innerHTML = '<span style="color:#cf1322">auto-login failed: ' + esc(String(e.message || e)) + '</span>';
      rows.innerHTML = '<tr><td colspan="7" style="color:#cf1322" class="muted">Auto-login failed. Check that user ' + AUTO_EMAIL + ' exists.</td></tr>';
    });
  </script>
</body>
</html>`
