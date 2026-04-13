package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	apiKey     string
	apiSecret  string
	configMu   sync.RWMutex
	port       string
	requestSeq uint64
	startedAt  = time.Now()
)

const (
	defaultPort       = "18491"
	defaultRecordType = "A"
	defaultTTL        = 600
	defaultConfigFile = "runtime_config.enc"
	readTimeout       = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	apiKey = strings.TrimSpace(os.Getenv("DNSHE_API_KEY"))
	apiSecret = strings.TrimSpace(os.Getenv("DNSHE_API_SECRET"))
	port = os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	if err := loadPersistentConfig(); err != nil {
		addLog("WARN", "load persistent config failed: %v", err)
	}

	addLog("INFO", "dnshe-ddns-go-callback starting on :%s (api_key=%v, api_secret=%v)",
		port, apiKey != "", apiSecret != "")

	http.HandleFunc("/", handleStatus)
	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/config", handleConfig)
	http.HandleFunc("/update", handleUpdate)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      nil,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
	log.Fatal(srv.ListenAndServe())
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	status := buildStatus()
	if r.URL.Query().Get("format") == "json" || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json") {
		writeJSON(w, http.StatusOK, status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := statusPageTemplate.Execute(w, status); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":  false,
			"msg": "render status page failed",
		})
	}
}

func buildStatus() map[string]any {
	logs := getLogs()
	uptime := int(time.Since(startedAt).Seconds())
	key, secret := getRuntimeConfig()
	return map[string]any{
		"ok":                    true,
		"service":               "dnshe-ddns-go-callback",
		"port":                  port,
		"uptime_seconds":        uptime,
		"api_key_configured":    key != "",
		"api_secret_configured": secret != "",
		"api_key_masked":        maskSecret(key),
		"api_secret_masked":     maskSecret(secret),
		"log_count":             len(logs),
		"logs":                  logs,
	}
}

type ConfigRequest struct {
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok":  false,
			"msg": "method not allowed",
		})
		return
	}

	var req ConfigRequest
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"ok":  false,
				"msg": "invalid json body",
			})
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"ok":  false,
				"msg": "invalid form body",
			})
			return
		}
		req.APIKey = r.FormValue("api_key")
		req.APISecret = r.FormValue("api_secret")
	}

	req.APIKey = strings.TrimSpace(req.APIKey)
	req.APISecret = strings.TrimSpace(req.APISecret)

	if req.APIKey == "" || req.APISecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":  false,
			"msg": "api_key and api_secret are required",
		})
		return
	}
	if err := savePersistentConfig(req.APIKey, req.APISecret); err != nil {
		addLog("ERROR", "save persistent config failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":  false,
			"msg": "保存配置失败，请确认 CONFIG_MASTER_KEY 已设置且目录可写",
		})
		return
	}

	configMu.Lock()
	apiKey = req.APIKey
	apiSecret = req.APISecret
	configMu.Unlock()

	addLog("INFO", "runtime config updated (api_key=true, api_secret=true)")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"msg":     "配置已生效",
		"updated": true,
	})
}

var statusPageTemplate = template.Must(template.New("status").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>dnshe-ddns-go-callback 状态</title>
  <style>
    :root { --line: #e6e8eb; --text: #1f2328; --muted: #57606a; --ok: #1f883d; --warn: #9a6700; --term-bg: #11161c; --term-line: #1d2630; --term-text: #d3d9e0; }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif;
      color: var(--text);
      background: #f7f8fa;
      line-height: 1.5;
    }
    .wrap { max-width: 920px; margin: 24px auto; padding: 0 16px; }
    .panel {
      background: #fff;
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 16px;
    }
    h1 { font-size: 22px; margin-bottom: 6px; }
    .sub { color: var(--muted); font-size: 13px; margin-bottom: 14px; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 10px; }
    .item { border: 1px solid var(--line); border-radius: 8px; padding: 10px 12px; background: #fcfcfd; }
    .label { color: var(--muted); font-size: 12px; }
    .value { font-size: 16px; margin-top: 4px; }
    .ok { color: var(--ok); }
    .warn { color: var(--warn); }
    .actions { margin-top: 12px; color: var(--muted); font-size: 13px; }
    .actions .sync { color: #656d76; }
    .cfg {
      margin-top: 12px;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 10px 12px;
      background: #fcfcfd;
    }
    .cfg-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
    .cfg-title { font-size: 13px; color: var(--muted); }
    .cfg-toggle {
      border: 1px solid #d0d7de;
      background: #fff;
      color: #1f2328;
      border-radius: 6px;
      padding: 4px 8px;
      font-size: 12px;
      cursor: pointer;
    }
    .cfg-summary { font-size: 12px; color: var(--muted); margin-bottom: 8px; }
    .cfg-row { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
    .cfg input {
      width: 100%;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      padding: 7px 9px;
      font-size: 13px;
    }
    .cfg button {
      margin-top: 8px;
      border: 1px solid #1f883d;
      background: #1f883d;
      color: #fff;
      border-radius: 6px;
      padding: 7px 10px;
      font-size: 13px;
      cursor: pointer;
    }
    .cfg-msg { margin-left: 8px; font-size: 12px; color: var(--muted); }
    .hidden { display: none; }
    .logs {
      margin-top: 14px;
      border: 1px solid var(--line);
      border-radius: 10px;
      background: #fff;
      overflow: hidden;
    }
    .loghead { padding: 10px 12px; border-bottom: 1px solid var(--line); font-weight: 600; background: #fafbfc; display: flex; justify-content: space-between; }
    .term {
      background: var(--term-bg);
      max-height: 420px;
      min-height: 240px;
      overflow: auto;
    }
    .logitem {
      display: grid;
      grid-template-columns: 72px 56px 1fr;
      gap: 10px;
      padding: 8px 12px;
      border-bottom: 1px solid var(--term-line);
      font: 13px/1.45 "SFMono-Regular", Menlo, Consolas, monospace;
    }
    .logitem:last-child { border-bottom: 0; }
    .time { color: #7e8a97; }
    .level { color: #6cb6ff; }
    .msg { color: var(--term-text); word-break: break-word; }
    .empty { padding: 12px; color: #93a1af; font-size: 13px; font-family: "SFMono-Regular", Menlo, Consolas, monospace; }
    .err { color: #ff8f8f; }
    .warnl { color: #f2cc60; }
  </style>
</head>
<body>
  <div class="wrap">
    <section class="panel">
      <h1>dnshe-ddns-go-callback</h1>
      <div class="sub">服务状态</div>
      <div class="grid">
        <div class="item">
          <div class="label">端口</div>
          <div class="value" id="port">{{index . "port"}}</div>
        </div>
        <div class="item">
          <div class="label">运行时长</div>
          <div class="value" id="uptime">{{index . "uptime_seconds"}}s</div>
        </div>
      </div>
      <div class="actions"><span class="sync" id="sync">实时同步中...</span></div>
      <form class="cfg" id="config-form">
        <div class="cfg-head">
          <div class="cfg-title">DNSHE API 配置（无需重启）</div>
          <button class="cfg-toggle hidden" type="button" id="cfg-toggle">修改配置</button>
        </div>
        <div class="cfg-summary" id="cfg-summary"></div>
        <div class="cfg-row" id="cfg-fields">
          <input id="api-key-input" name="api_key" placeholder="DNSHE_API_KEY" autocomplete="off" />
          <input id="api-secret-input" name="api_secret" placeholder="DNSHE_API_SECRET" autocomplete="off" />
        </div>
        <button type="submit" id="cfg-submit">保存并生效</button>
        <span class="cfg-msg" id="cfg-msg"></span>
      </form>
    </section>

    <section class="logs">
      <div class="loghead">
        <span>最近日志（<span id="log-count">{{index . "log_count"}}</span>）</span>
        <span id="last-updated">--:--:--</span>
      </div>
      <div class="term" id="log-panel">
        <div id="log-list">
          {{- $logs := index . "logs" -}}
          {{- if $logs }}
            {{- range $logs }}
              <div class="logitem">
                <div class="time">{{.Time}}</div>
                <div class="level">{{.Level}}</div>
                <div class="msg">{{.Message}}</div>
              </div>
            {{- end }}
          {{- else }}
            <div class="empty">暂无日志</div>
          {{- end }}
        </div>
      </div>
    </section>
  </div>
  <script>
    (function () {
      const url = "/status?format=json";
      const syncEl = document.getElementById("sync");
      const logPanel = document.getElementById("log-panel");
      const logList = document.getElementById("log-list");
      const logCount = document.getElementById("log-count");
      const lastUpdated = document.getElementById("last-updated");
      const uptime = document.getElementById("uptime");
      const configForm = document.getElementById("config-form");
      const cfgMsg = document.getElementById("cfg-msg");
      const cfgSummary = document.getElementById("cfg-summary");
      const cfgToggle = document.getElementById("cfg-toggle");
      const cfgFields = document.getElementById("cfg-fields");
      const cfgSubmit = document.getElementById("cfg-submit");
      const keyInput = document.getElementById("api-key-input");
      const secretInput = document.getElementById("api-secret-input");
      let editingConfig = false;

      function esc(s) {
        return String(s || "")
          .replaceAll("&", "&amp;")
          .replaceAll("<", "&lt;")
          .replaceAll(">", "&gt;")
          .replaceAll('"', "&quot;")
          .replaceAll("'", "&#39;");
      }

      function statusHtml(ok, labelOk, labelBad) {
        return ok ? '<span class="ok">' + labelOk + "</span>" : '<span class="warn">' + labelBad + "</span>";
      }

      function setConfigView(data) {
        const configured = !!data.api_key_configured && !!data.api_secret_configured;
        if (configured && !editingConfig) {
          cfgSummary.innerHTML = "当前状态：" + statusHtml(true, "已配置", "未配置") + "（已做脱敏展示）";
          keyInput.value = String(data.api_key_masked || "");
          secretInput.value = String(data.api_secret_masked || "");
          keyInput.readOnly = true;
          secretInput.readOnly = true;
          cfgSubmit.classList.add("hidden");
          cfgToggle.classList.remove("hidden");
        } else {
          cfgSummary.innerHTML = configured
            ? "修改模式：请输入新的 Key / Secret"
            : "当前状态：" + statusHtml(false, "已配置", "未配置") + "，请先填写 Key 与 Secret。";
          keyInput.readOnly = false;
          secretInput.readOnly = false;
          cfgSubmit.classList.remove("hidden");
          cfgToggle.classList.toggle("hidden", !configured || editingConfig);
        }
      }

      function renderLogs(logs) {
        if (!Array.isArray(logs) || logs.length === 0) {
          logList.innerHTML = '<div class="empty">暂无日志</div>';
          return;
        }
        const nearBottom = logPanel.scrollHeight - logPanel.scrollTop - logPanel.clientHeight < 80;
        logList.innerHTML = logs.map(function (l) {
          const level = esc(l.level);
          let levelClass = "";
          if (level === "ERROR") levelClass = " err";
          if (level === "WARN") levelClass = " warnl";
          return '<div class="logitem">' +
            '<div class="time">' + esc(l.time) + "</div>" +
            '<div class="level' + levelClass + '">' + level + "</div>" +
            '<div class="msg">' + esc(l.message) + "</div>" +
          "</div>";
        }).join("");
        if (nearBottom) {
          logPanel.scrollTop = logPanel.scrollHeight;
        }
      }

      async function refresh() {
        try {
          const res = await fetch(url, { cache: "no-store" });
          if (!res.ok) throw new Error("HTTP " + res.status);
          const data = await res.json();
          uptime.textContent = String(data.uptime_seconds || 0) + "s";
          setConfigView(data);
          logCount.textContent = String(data.log_count || 0);
          renderLogs(data.logs);
          const t = new Date();
          lastUpdated.textContent = t.toLocaleTimeString("zh-CN", { hour12: false });
          syncEl.textContent = "实时同步中（每2秒）";
          syncEl.style.color = "#57606a";
        } catch (e) {
          syncEl.textContent = "同步失败：" + (e && e.message ? e.message : "未知错误");
          syncEl.style.color = "#b62324";
        }
      }

      configForm.addEventListener("submit", async function (e) {
        e.preventDefault();
        cfgMsg.textContent = "保存中...";
        cfgMsg.style.color = "#57606a";
        try {
          const body = new URLSearchParams();
          body.set("api_key", keyInput.value.trim());
          body.set("api_secret", secretInput.value.trim());
          const res = await fetch("/config", {
            method: "POST",
            headers: { "Content-Type": "application/x-www-form-urlencoded" },
            body: body.toString(),
          });
          const data = await res.json();
          if (!res.ok || !data.ok) {
            throw new Error(data.msg || ("HTTP " + res.status));
          }
          cfgMsg.textContent = "已生效";
          cfgMsg.style.color = "#1f883d";
          editingConfig = false;
          refresh();
        } catch (e2) {
          cfgMsg.textContent = "失败：" + (e2 && e2.message ? e2.message : "未知错误");
          cfgMsg.style.color = "#b62324";
        }
      });

      cfgToggle.addEventListener("click", function () {
        editingConfig = true;
        keyInput.value = "";
        secretInput.value = "";
        keyInput.focus();
        cfgSubmit.classList.remove("hidden");
        setConfigView({ api_key_configured: true, api_secret_configured: true });
      });

      refresh();
      setInterval(refresh, 2000);
    })();
  </script>
</body>
</html>`))

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	reqID := nextRequestID()

	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, reqID, "method not allowed, use GET or POST")
		return
	}
	if !hasDNSHEConfig() {
		writeError(w, http.StatusServiceUnavailable, reqID, "DNSHE API not configured, please set API Key/Secret in /status")
		return
	}

	req, err := parseUpdateRequest(r)
	if err != nil {
		addLog("ERROR", "[req=%s] parse failed: %v", reqID, err)
		writeError(w, http.StatusBadRequest, reqID, err.Error())
		return
	}

	if req.Domain == "" || req.IP == "" {
		writeError(w, http.StatusBadRequest, reqID, "missing domain or ip")
		return
	}

	if req.RecordType == "" {
		req.RecordType = defaultRecordType
	}
	if req.TTL == 0 {
		req.TTL = defaultTTL
	}
	req.RecordType = strings.ToUpper(strings.TrimSpace(req.RecordType))
	if req.RecordType != "A" && req.RecordType != "AAAA" {
		writeError(w, http.StatusBadRequest, reqID, "recordType must be A or AAAA")
		return
	}

	addLog("INFO", "[req=%s] domain=%s type=%s ip=%s ttl=%d",
		reqID, req.Domain, req.RecordType, maskIP(req.IP), req.TTL)

	// 1. 查子域名
	subID, err := FindSubdomain(req.Domain)
	if err != nil {
		addLog("ERROR", "[req=%s] find subdomain: %v", reqID, err)
		writeError(w, http.StatusBadGateway, reqID, "failed to query DNSHE subdomains: "+err.Error())
		return
	}
	if subID == 0 {
		writeError(w, http.StatusNotFound, reqID, "subdomain not found: "+req.Domain)
		return
	}

	// 2. 查记录
	recordID, oldIP, err := FindRecord(subID, req.RecordType)
	if err != nil {
		addLog("ERROR", "[req=%s] find record: %v", reqID, err)
		writeError(w, http.StatusBadGateway, reqID, "failed to query DNSHE records: "+err.Error())
		return
	}
	if recordID == 0 {
		writeError(w, http.StatusNotFound, reqID, "record not found: "+req.RecordType)
		return
	}

	// 3. IP 没变，跳过
	if oldIP == req.IP {
		addLog("INFO", "[req=%s] ip unchanged: %s", reqID, maskIP(req.IP))
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"msg":        "record unchanged, update skipped",
			"request_id": reqID,
		})
		return
	}

	// 4. 更新
	result, err := UpdateRecord(recordID, req.IP, req.TTL)
	if err != nil {
		addLog("ERROR", "[req=%s] update record: %v", reqID, err)
		writeError(w, http.StatusBadGateway, reqID, "failed to update DNSHE record: "+err.Error())
		return
	}

	addLog("INFO", "[req=%s] updated: %s %s -> %s (record_id=%d)",
		reqID, req.Domain, maskIP(oldIP), maskIP(req.IP), recordID)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"result":     result,
		"request_id": reqID,
	})
}

func parseUpdateRequest(r *http.Request) (UpdateRequest, error) {
	var req UpdateRequest

	// 先尝试 URL query，兼容 GET 和 POST 场景
	fillUpdateRequestFromValues(&req, r.URL.Query())

	bodyBytes, _ := io.ReadAll(r.Body)
	bodyStr := strings.TrimSpace(string(bodyBytes))
	ct := r.Header.Get("Content-Type")
	addLog("DEBUG", "method=%s content-type=%s query=%q body=%s", r.Method, ct, r.URL.RawQuery, bodyStr)

	if bodyStr == "" {
		if req.Domain == "" && req.IP == "" {
			return req, fmt.Errorf("empty request")
		}
		return req, nil
	}

	// 1) JSON
	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err == nil {
		fillUpdateRequestFromMap(&req, raw)
	} else {
		// 2) x-www-form-urlencoded / query-like
		vals, err2 := url.ParseQuery(bodyStr)
		if err2 == nil {
			fillUpdateRequestFromValues(&req, vals)
		}
		// 3) 容错提取：某些 ddns-go 配置会产生不完整 JSON（缺少结尾 } 或多余引号）
		if req.Domain == "" || req.IP == "" {
			fillUpdateRequestFromLooseJSON(&req, bodyStr)
		}
	}

	if strings.Contains(req.Domain, "#{") || strings.Contains(req.IP, "#{") {
		return req, fmt.Errorf("ddns-go placeholders were not expanded, check callback template")
	}
	if req.Domain == "" && req.IP == "" {
		return req, fmt.Errorf("invalid request body, expected JSON or form fields: domain/ip")
	}

	req.Domain = strings.TrimSpace(req.Domain)
	req.IP = strings.TrimSpace(req.IP)
	req.RecordType = strings.TrimSpace(req.RecordType)

	return req, nil
}

func fillUpdateRequestFromValues(req *UpdateRequest, vals url.Values) {
	if req.Domain == "" {
		req.Domain = firstNonEmpty(vals.Get("domain"), vals.Get("full_domain"), vals.Get("host"))
	}
	if req.IP == "" {
		req.IP = firstNonEmpty(vals.Get("ip"), vals.Get("value"), vals.Get("content"))
	}
	if req.RecordType == "" {
		req.RecordType = firstNonEmpty(vals.Get("recordType"), vals.Get("record_type"), vals.Get("type"))
	}
	if req.TTL == 0 {
		ttlStr := firstNonEmpty(vals.Get("ttl"), vals.Get("TTL"))
		req.TTL = parseTTL(ttlStr)
	}
}

func fillUpdateRequestFromMap(req *UpdateRequest, m map[string]any) {
	if req.Domain == "" {
		req.Domain = firstNonEmpty(stringFromAny(m["domain"]), stringFromAny(m["full_domain"]), stringFromAny(m["host"]))
	}
	if req.IP == "" {
		req.IP = firstNonEmpty(stringFromAny(m["ip"]), stringFromAny(m["value"]), stringFromAny(m["content"]))
	}
	if req.RecordType == "" {
		req.RecordType = firstNonEmpty(
			stringFromAny(m["recordType"]),
			stringFromAny(m["record_type"]),
			stringFromAny(m["type"]),
		)
	}
	if req.TTL == 0 {
		ttlStr := firstNonEmpty(stringFromAny(m["ttl"]), stringFromAny(m["TTL"]))
		req.TTL = parseTTL(ttlStr)
	}
}

func parseTTL(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func fillUpdateRequestFromLooseJSON(req *UpdateRequest, body string) {
	if req.Domain == "" {
		req.Domain = firstNonEmpty(
			findLooseJSONValue(body, "domain"),
			findLooseJSONValue(body, "full_domain"),
			findLooseJSONValue(body, "host"),
		)
	}
	if req.IP == "" {
		req.IP = firstNonEmpty(
			findLooseJSONValue(body, "ip"),
			findLooseJSONValue(body, "value"),
			findLooseJSONValue(body, "content"),
		)
	}
	if req.RecordType == "" {
		req.RecordType = firstNonEmpty(
			findLooseJSONValue(body, "recordType"),
			findLooseJSONValue(body, "record_type"),
			findLooseJSONValue(body, "type"),
		)
	}
	if req.TTL == 0 {
		req.TTL = parseTTL(firstNonEmpty(
			findLooseJSONValue(body, "ttl"),
			findLooseJSONValue(body, "TTL"),
		))
	}
}

func findLooseJSONValue(body, key string) string {
	pat := fmt.Sprintf(`"%s"\s*:\s*"([^"]*)"`, regexp.QuoteMeta(key))
	re := regexp.MustCompile(pat)
	m := re.FindStringSubmatch(body)
	if len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func stringFromAny(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%.0f", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	n := len(s)
	if n == 0 {
		return ""
	}
	if n <= 4 {
		return strings.Repeat("*", n)
	}
	if n <= 8 {
		return s[:1] + "***" + s[n-1:]
	}
	return s[:3] + "***" + s[n-3:]
}

func writeError(w http.ResponseWriter, status int, requestID, msg string) {
	writeJSON(w, status, map[string]any{
		"ok":         false,
		"msg":        msg,
		"request_id": requestID,
	})
}

func nextRequestID() string {
	n := atomic.AddUint64(&requestSeq, 1)
	return fmt.Sprintf("r-%d", n)
}

func getRuntimeConfig() (key, secret string) {
	configMu.RLock()
	defer configMu.RUnlock()
	return apiKey, apiSecret
}

func hasDNSHEConfig() bool {
	key, secret := getRuntimeConfig()
	return key != "" && secret != ""
}

func runtimeConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("CONFIG_PATH")); p != "" {
		return p
	}
	return defaultConfigFile
}

func loadPersistentConfig() error {
	path := runtimeConfigPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cfg, err := decryptConfig(b)
	if err != nil {
		return err
	}
	if cfg.APIKey == "" || cfg.APISecret == "" {
		return nil
	}

	configMu.Lock()
	if apiKey == "" {
		apiKey = cfg.APIKey
	}
	if apiSecret == "" {
		apiSecret = cfg.APISecret
	}
	configMu.Unlock()
	return nil
}

func savePersistentConfig(key, secret string) error {
	plain := ConfigRequest{
		APIKey:    strings.TrimSpace(key),
		APISecret: strings.TrimSpace(secret),
	}
	b, err := encryptConfig(plain)
	if err != nil {
		return err
	}

	path := runtimeConfigPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeJSON 写 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// readJSON 读 JSON 请求体
func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
