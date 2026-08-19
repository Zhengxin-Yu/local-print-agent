package web

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This executes the shipped JavaScript against a small DOM boundary. Unlike
// the static contract checks, syntax errors, wrong fetch sequencing, unsafe
// text rendering, broken sorting, or a missing unload cleanup fail by behavior.
func TestAppJavaScriptRunsDiscoveryRenderingAndTimerLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is unavailable; portable static web contracts still run")
	}
	source, err := fs.ReadFile(Assets, "app.js")
	if err != nil {
		t.Fatal(err)
	}
	appPath := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(appPath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, "-e", browserBehaviorHarness, appPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("execute app.js behavior contract: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "behavior ok") {
		t.Fatalf("unexpected Node.js output: %s", output)
	}
}

func TestAppJavaScriptPropagatesFileOriginCapabilityToRequestsAndPreview(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is unavailable; portable static web contracts still run")
	}
	source, err := fs.ReadFile(Assets, "app.js")
	if err != nil {
		t.Fatal(err)
	}
	appPath := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(appPath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, "-e", fileOriginCapabilityHarness, appPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("execute file-origin app.js behavior contract: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "file capability ok") {
		t.Fatalf("unexpected Node.js output: %s", output)
	}
}

const browserBehaviorHarness = `
const fs = require("fs");
const vm = require("vm");
const appPath = process.argv[process.argv.length - 1];
const nativeSetTimeout = global.setTimeout;

class Element {
  constructor(id = "") { this.id = id; this.children = []; this.listeners = {}; this.textContent = ""; this.hidden = true; this.value = ""; this.disabled = false; }
  append(...items) { this.children.push(...items); }
  replaceChildren(...items) { this.children = [...items]; }
  addEventListener(name, callback) { this.listeners[name] = callback; }
  querySelector() { return new Element("submit"); }
}
const elements = new Map();
const byId = (id) => { if (!elements.has(id)) elements.set(id, new Element(id)); return elements.get(id); };
for (const id of ["page-error", "connection-status", "api-version", "service-port", "jobs-body", "job-detail", "printer-select", "printer-hint", "balloon-form", "source-form", "refresh-jobs", "balloon-solved-at", "balloon-team-name", "balloon-problem-id", "source-language", "source-code"]) byId(id);
global.document = { getElementById: byId, createElement: () => new Element() };

const windowListeners = {};
let intervalDelay = 0;
let clearedInterval = 0;
global.window = {
  location: { protocol: "http:", origin: "http://127.0.0.1:17654" },
  addEventListener: (name, callback) => { windowListeners[name] = callback; },
  setInterval: (_callback, delay) => { intervalDelay = delay; return 17; },
  clearInterval: (id) => { clearedInterval = id; },
};

const calls = [];
const response = (data) => ({ ok: true, status: 200, json: async () => data });
global.fetch = async (url) => {
  calls.push(url);
  if (url.endsWith("/health")) return response({ service: "local-print-agent", api_version: "v1", status: "ok" });
  if (url.endsWith("/api/v1/printers")) return response({ data: [{ name: "Mock Printer", is_default: true }], error: null });
  if (url.endsWith("/api/v1/print-jobs")) return response({ data: [
    { id: "older", created_at: "2026-08-19T08:00:00Z", type: "source_code", printer_name: "Mock Printer", status: "failed", error: { message: "<img src=x onerror=1>" } },
    { id: "newer", created_at: "2026-08-19T09:00:00Z", type: "balloon_ticket", printer_name: "Mock Printer", status: "succeeded", error: null },
  ], error: null });
  throw new Error("unexpected fetch " + url);
};

function assert(condition, message) { if (!condition) throw new Error(message); }
vm.runInThisContext(fs.readFileSync(appPath, "utf8"), { filename: appPath });

nativeSetTimeout(() => {
  try {
    assert(byId("connection-status").textContent === "已连接", "health discovery did not connect");
    assert(calls[0] === "http://127.0.0.1:17654/health", "same-origin health was not first");
    assert(byId("printer-select").children[0].value === "Mock Printer", "printer list was not rendered");
    const rows = byId("jobs-body").children;
    assert(rows.length === 2, "job rows were not rendered");
    assert(rows[0].children[1].textContent === "balloon_ticket", "jobs were not sorted newest first");
    assert(rows[1].children[4].textContent === "<img src=x onerror=1>", "error text was not preserved as textContent");
    assert(intervalDelay === 2000, "poll interval was not 2000ms");
    windowListeners.beforeunload();
    assert(clearedInterval === 17, "unload did not clear polling timer");
    process.stdout.write("behavior ok\n");
  } catch (error) {
    console.error(error.stack || error);
    process.exitCode = 1;
  }
}, 25);
`

const fileOriginCapabilityHarness = `
const fs = require("fs");
const vm = require("vm");
const appPath = process.argv[process.argv.length - 1];
const nativeSetTimeout = global.setTimeout;
const token = "launch+capability";

class Element {
  constructor(id = "") { this.id = id; this.children = []; this.listeners = {}; this.textContent = ""; this.hidden = true; this.value = ""; this.disabled = false; }
  append(...items) { this.children.push(...items); }
  replaceChildren(...items) { this.children = [...items]; }
  addEventListener(name, callback) { this.listeners[name] = callback; }
  querySelector() { return new Element("submit"); }
  reportValidity() { return true; }
}
const elements = new Map();
const byId = (id) => { if (!elements.has(id)) elements.set(id, new Element(id)); return elements.get(id); };
for (const id of ["page-error", "connection-status", "api-version", "service-port", "jobs-body", "job-detail", "printer-select", "printer-hint", "balloon-form", "source-form", "refresh-jobs", "balloon-solved-at", "balloon-team-name", "balloon-problem-id", "source-language", "source-code"]) byId(id);
byId("balloon-solved-at").value = "2026-08-19T09:30";
byId("balloon-team-name").value = "Team";
byId("balloon-problem-id").value = "A";
global.document = { getElementById: byId, createElement: () => new Element() };

global.window = {
  location: { protocol: "file:", origin: "null", search: "?local_print_agent_token=launch%2Bcapability" },
  addEventListener: () => {},
  setInterval: () => 17,
  clearInterval: () => {},
};

const calls = [];
const response = (data) => ({ ok: true, status: 200, json: async () => data });
global.fetch = async (url, options = {}) => {
  const parsed = new URL(url);
  calls.push({ parsed, options });
  if (parsed.searchParams.get("local_print_agent_token") !== token) throw new Error("missing file-origin capability: " + url);
  if (parsed.pathname === "/health") return response({ service: "local-print-agent", api_version: "v1", status: "ok" });
  if (parsed.pathname === "/api/v1/printers") return response({ data: [{ name: "Mock Printer", is_default: true }], error: null });
  if (parsed.pathname === "/api/v1/print-jobs/job1/retry") return response({ data: { id: "job1" }, error: null });
  if (parsed.pathname === "/api/v1/print-jobs/job1") return response({ data: { id: "job1", pdf_path: "data/jobs/job1/preview.pdf" }, error: null });
  if (parsed.pathname === "/api/v1/print-jobs" && options.method === "POST") return response({ data: { id: "created" }, error: null });
  if (parsed.pathname === "/api/v1/print-jobs") return response({ data: [{ id: "job1", created_at: "2026-08-19T09:00:00Z", type: "balloon_ticket", printer_name: "Mock Printer", status: "failed", error: { message: "failed" } }], error: null });
  throw new Error("unexpected fetch " + url);
};

function assert(condition, message) { if (!condition) throw new Error(message); }
vm.runInThisContext(fs.readFileSync(appPath, "utf8"), { filename: appPath });

nativeSetTimeout(async () => {
  try {
    assert(byId("connection-status").textContent === "已连接", "file console did not connect with its capability");
    const firstRow = byId("jobs-body").children[0];
    await firstRow.children[5].children[0].listeners.click();
    const preview = byId("job-detail").children[1];
    assert(new URL(preview.href).searchParams.get("local_print_agent_token") === token, "preview URL omitted the file-origin capability");
    await firstRow.children[5].children[1].listeners.click();
    byId("balloon-form").listeners.submit({ preventDefault: () => {}, currentTarget: byId("balloon-form") });
    await new Promise((resolve) => nativeSetTimeout(resolve, 25));
    const paths = new Set(calls.map((call) => call.parsed.pathname));
    for (const required of ["/health", "/api/v1/printers", "/api/v1/print-jobs", "/api/v1/print-jobs/job1", "/api/v1/print-jobs/job1/retry"]) assert(paths.has(required), "missing request " + required);
    for (const call of calls) assert(call.parsed.searchParams.get("local_print_agent_token") === token, "request omitted capability: " + call.parsed.href);
    process.stdout.write("file capability ok\n");
  } catch (error) {
    console.error(error.stack || error);
    process.exitCode = 1;
  }
}, 50);
`
