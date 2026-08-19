(() => {
  "use strict";
  const byId = (id) => document.getElementById(id);
  const state = { origin: "", refreshTimer: null, busy: new Set() };
  const errorBox = byId("page-error");

  function showError(message) { errorBox.textContent = message; errorBox.hidden = false; }
  function clearError() { errorBox.textContent = ""; errorBox.hidden = true; }
  function setConnection(text, version, origin) {
    byId("connection-status").textContent = text;
    byId("api-version").textContent = version || "—";
    try { byId("service-port").textContent = origin ? new URL(origin).port || "默认端口" : "—"; } catch (_) { byId("service-port").textContent = "—"; }
  }
  async function fetchJSON(path, options = {}) {
    const response = await fetch(state.origin + path, { ...options, headers: { "Content-Type": "application/json", ...(options.headers || {}) } });
    let result;
    try { result = await response.json(); } catch (_) { throw new Error("服务返回了无效响应。"); }
    if (!response.ok || result.error) { throw new Error(result.error && result.error.message ? result.error.message : `请求失败（${response.status}）`); }
    return result.data;
  }
  async function health(origin) {
    const controller = new AbortController(); const timeout = setTimeout(() => controller.abort(), 1000);
    try {
      const response = await fetch(origin + "/health", { signal: controller.signal });
      const data = await response.json();
      return response.ok && data.service === "local-print-agent" && data.api_version === "v1" && data.status === "ok";
    } catch (_) { return false; } finally { clearTimeout(timeout); }
  }
  async function discoverOrigin() {
    if (window.location.protocol !== "file:") {
      return (await health(window.location.origin)) ? window.location.origin : "";
    }
    for (let port = 17653; port <= 17660; port += 1) { const origin = `http://127.0.0.1:${port}`; if (await health(origin)) return origin; }
    return "";
  }
  function setBusy(button, busy) { button.disabled = busy; if (busy) state.busy.add(button); else state.busy.delete(button); }
  function statusCell(job) { const span = document.createElement("span"); const pending = ["queued", "rendering", "printing"].includes(job.status); span.className = `status ${job.status === "succeeded" ? "status-succeeded" : job.status === "failed" ? "status-failed" : "status-pending"}`; span.textContent = `${pending ? "进行中：" : ""}${job.status}`; return span; }
  function textCell(text) { const td = document.createElement("td"); td.textContent = text || "—"; return td; }
  function formatTime(value) { const time = new Date(value); return Number.isNaN(time.getTime()) ? "—" : time.toLocaleString("zh-CN", { hour12:false }); }
  function renderJobs(jobs) {
    const body = byId("jobs-body"); body.replaceChildren();
    jobs.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
    if (!jobs.length) { const row = document.createElement("tr"); const cell = document.createElement("td"); cell.colSpan = 6; cell.textContent = "尚无打印任务。"; row.append(cell); body.append(row); return; }
    for (const job of jobs) {
      const row = document.createElement("tr"); row.append(textCell(formatTime(job.created_at)), textCell(job.type), textCell(job.printer_name));
      const status = document.createElement("td"); status.append(statusCell(job)); row.append(status);
      row.append(textCell(job.error && job.error.message));
      const actions = document.createElement("td"); actions.className = "row-actions";
      const details = document.createElement("button"); details.type = "button"; details.textContent = "详情"; details.addEventListener("click", () => loadDetail(job.id, details)); actions.append(details);
      if (job.status === "failed") { const retry = document.createElement("button"); retry.type = "button"; retry.textContent = "重试"; retry.addEventListener("click", () => retryJob(job.id, retry)); actions.append(retry); }
      row.append(actions); body.append(row);
    }
  }
  async function refreshJobs(button) { if (!state.origin) return; if (button) setBusy(button, true); try { renderJobs(await fetchJSON("/api/v1/print-jobs", { headers: {} })); clearError(); } catch (error) { showError(`无法刷新任务：${error.message}`); } finally { if (button) setBusy(button, false); } }
  async function loadDetail(id, button) { setBusy(button, true); try { const job = await fetchJSON(`/api/v1/print-jobs/${encodeURIComponent(id)}`, { headers: {} }); const detail = byId("job-detail"); detail.replaceChildren(); const pre = document.createElement("pre"); pre.textContent = JSON.stringify(job, null, 2); detail.append(pre); if (job.pdf_path) { const link = document.createElement("a"); link.href = state.origin + `/api/v1/print-jobs/${encodeURIComponent(id)}/preview`; link.className = "preview-link"; link.textContent = "打开 PDF 预览（Mock 当前会返回未实现）"; link.addEventListener("click", async (event) => { event.preventDefault(); try { await fetchJSON(`/api/v1/print-jobs/${encodeURIComponent(id)}/preview`, { headers: {} }); } catch (error) { showError(`预览不可用：${error.message}`); } }); detail.append(link); } clearError(); } catch (error) { showError(`无法加载详情：${error.message}`); } finally { setBusy(button, false); } }
  async function retryJob(id, button) { setBusy(button, true); try { await fetchJSON(`/api/v1/print-jobs/${encodeURIComponent(id)}/retry`, { method:"POST" }); await refreshJobs(); clearError(); } catch (error) { showError(`重试失败：${error.message}`); } finally { setBusy(button, false); } }
  async function createJob(type, payload, button) { const printer = byId("printer-select").value; if (!printer) { showError("请先选择可用打印机。"); return; } setBusy(button, true); try { await fetchJSON("/api/v1/print-jobs", { method:"POST", body:JSON.stringify({ type, printer_name:printer, payload }) }); await refreshJobs(); clearError(); } catch (error) { showError(`创建任务失败：${error.message}`); } finally { setBusy(button, false); } }
  async function loadPrinters() { const select = byId("printer-select"); try { const printers = await fetchJSON("/api/v1/printers", { headers: {} }); select.replaceChildren(); if (!printers.length) { const option = document.createElement("option"); option.value = ""; option.textContent = "没有可用打印机"; select.append(option); byId("printer-hint").textContent = "打印机列表为空，暂时不能创建任务。"; return; } for (const printer of printers) { const option = document.createElement("option"); option.value = printer.name; option.textContent = `${printer.name}${printer.is_default ? "（默认）" : ""}`; select.append(option); } } catch (error) { select.replaceChildren(); const option = document.createElement("option"); option.value = ""; option.textContent = "打印机依赖不可用"; select.append(option); byId("printer-hint").textContent = "无法读取打印机：" + error.message; showError(`打印机不可用：${error.message}`); } }
  function bindForms() { byId("balloon-form").addEventListener("submit", (event) => { event.preventDefault(); const form = event.currentTarget; const time = byId("balloon-solved-at").value; if (!form.reportValidity()) return; createJob("balloon_ticket", { team_name:byId("balloon-team-name").value, problem_id:byId("balloon-problem-id").value, solved_at:new Date(time).toISOString() }, form.querySelector("button[type=submit]")); }); byId("source-form").addEventListener("submit", (event) => { event.preventDefault(); const form = event.currentTarget; if (!form.reportValidity()) return; createJob("source_code", { language:byId("source-language").value, source_code:byId("source-code").value }, form.querySelector("button[type=submit]")); }); byId("refresh-jobs").addEventListener("click", (event) => refreshJobs(event.currentTarget)); }
  async function start() { bindForms(); state.origin = await discoverOrigin(); if (!state.origin) { setConnection("未连接（请启动本地服务）", "—", ""); showError("未找到本地打印代理。请运行启动脚本后刷新页面。"); return; } setConnection("已连接", "v1", state.origin); await loadPrinters(); await refreshJobs(); state.refreshTimer = window.setInterval(() => refreshJobs(), 2000); }
  window.addEventListener("beforeunload", () => { if (state.refreshTimer) window.clearInterval(state.refreshTimer); }); start();
})();
