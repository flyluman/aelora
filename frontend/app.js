(() => {
  function newLoadTestState() {
    return {
      running: false,
      stopRequested: false,
      mode: "http",
      total: 0,
      nextIndex: 0,
      completed: 0,
      success: 0,
      failed: 0,
      inFlight: 0,
      startedAt: 0,
      finishedAt: 0,
      latencies: [],
      lastError: "",
      roomId: "",
      config: null,
      wsPending: new Map(),
    };
  }

  const state = {
    baseUrl: "",
    userId: "",
    deviceId: "",
    ws: null,
    wsConnected: false,
    wsConnecting: false,
    autoScroll: true,
    logs: [],
    nextLogID: 1,
    logLevel: "all",
    logSearch: "",
    busy: new Set(),
    toastTimer: null,
    activeTab: "rooms",
    loadTest: newLoadTestState(),
    loadTickTimer: null,
    loadRenderQueued: false,
    opsMetricsPrevious: null,
    opsRefreshTimer: null,
  };

  const $ = (id) => document.getElementById(id);

  function fallbackBaseURL() {
    if (window.location.origin && window.location.origin.startsWith("http")) {
      return normalizeBase(window.location.origin);
    }
    return "http://127.0.0.1:8081";
  }

  function nowISO() {
    return new Date().toISOString();
  }

  function safeJSON(value) {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }

  function average(values) {
    if (!values || values.length === 0) return 0;
    const total = values.reduce((sum, value) => sum + value, 0);
    return total / values.length;
  }

  function percentile(values, p) {
    if (!values || values.length === 0) return 0;
    const sorted = [...values].sort((a, b) => a - b);
    const index = Math.min(sorted.length - 1, Math.max(0, Math.ceil((p / 100) * sorted.length) - 1));
    return sorted[index];
  }

  function formatMilliseconds(ms) {
    if (!Number.isFinite(ms) || ms <= 0) return "0 ms";
    if (ms < 1000) {
      return `${ms >= 100 ? Math.round(ms) : ms.toFixed(1)} ms`;
    }
    return `${(ms / 1000).toFixed(2)} s`;
  }

  function formatBytes(bytes) {
    if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let value = bytes;
    let unitIndex = 0;
    while (value >= 1024 && unitIndex < units.length - 1) {
      value /= 1024;
      unitIndex += 1;
    }
    return `${value >= 100 ? Math.round(value) : value.toFixed(value >= 10 ? 1 : 2)} ${units[unitIndex]}`;
  }

  function formatCount(value) {
    if (!Number.isFinite(value)) return "0";
    return new Intl.NumberFormat().format(Math.round(value));
  }

  function formatDurationSeconds(seconds) {
    if (!Number.isFinite(seconds) || seconds <= 0) return "0 s";
    if (seconds < 60) return `${seconds.toFixed(seconds >= 10 ? 0 : 1)} s`;
    if (seconds < 3600) return `${(seconds / 60).toFixed(1)} min`;
    return `${(seconds / 3600).toFixed(1)} h`;
  }

  function formatPercent(value) {
    if (!Number.isFinite(value)) return "sampling...";
    return `${value.toFixed(value >= 10 ? 0 : 1)}%`;
  }

  function formatMetricSeconds(seconds) {
    if (!Number.isFinite(seconds) || seconds <= 0) return "0 s";
    if (seconds < 10) return `${seconds.toFixed(2)} s`;
    if (seconds < 100) return `${seconds.toFixed(1)} s`;
    return `${Math.round(seconds)} s`;
  }

  function loadTransportLabel(mode) {
    return mode === "ws" ? "WebSocket" : "HTTP";
  }

  function metricsPreview(text, maxLines = 40) {
    const lines = String(text || "")
      .split("\n")
      .filter((line) => line.trim() !== "");
    return {
      lineCount: lines.length,
      preview: lines.slice(0, maxLines).join("\n"),
    };
  }

  function parsePrometheusMetrics(text) {
    const metrics = Object.create(null);
    const pattern = /^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{[^}]*\})?\s+([+-]?(?:\d+(?:\.\d+)?|\.\d+)(?:[eE][+-]?\d+)?)$/;

    for (const rawLine of String(text || "").split("\n")) {
      const line = rawLine.trim();
      if (!line || line.startsWith("#")) continue;
      const match = line.match(pattern);
      if (!match) continue;
      const value = Number(match[2]);
      if (Number.isFinite(value)) {
        metrics[match[1]] = value;
      }
    }

    return metrics;
  }

  function renderRuntimeMetrics(metrics, sampledAt = Date.now()) {
    const cpuSeconds = Number(metrics.process_cpu_seconds_total || 0);
    const uptimeSeconds = Number(metrics.process_uptime_seconds || 0);
    const heapBytes = Number(metrics.go_memory_heap_objects_bytes || 0);
    const totalMemoryBytes = Number(metrics.go_memory_total_bytes || 0);
    const goroutines = Number(metrics.go_goroutines || 0);
    const threads = Number(metrics.go_threads || 0);
    const gcCycles = Number(metrics.go_gc_cycles_total || 0);
    const requests = Number(metrics.aelora_http_requests_total || 0);
    const gomaxprocs = Math.max(1, Number(metrics.go_gomaxprocs || 1));

    let cpuUsagePercent = Number.NaN;
    const previous = state.opsMetricsPrevious;
    if (previous && cpuSeconds >= previous.cpuSeconds && sampledAt > previous.sampledAt) {
      const cpuDelta = cpuSeconds - previous.cpuSeconds;
      const wallSeconds = (sampledAt - previous.sampledAt) / 1000;
      if (wallSeconds > 0) {
        cpuUsagePercent = Math.max(0, (cpuDelta / wallSeconds) * (100 / gomaxprocs));
      }
    }

    state.opsMetricsPrevious = { cpuSeconds, sampledAt, gomaxprocs };

    $("opsCpuUsage").textContent = formatPercent(cpuUsagePercent);
    $("opsCpuSeconds").textContent = formatMetricSeconds(cpuSeconds);
    $("opsMemoryHeap").textContent = formatBytes(heapBytes);
    $("opsMemoryTotal").textContent = formatBytes(totalMemoryBytes);
    $("opsGoroutines").textContent = formatCount(goroutines);
    $("opsThreads").textContent = formatCount(threads);
    $("opsGC").textContent = formatCount(gcCycles);
    $("opsRequests").textContent = formatCount(requests);
    $("opsUptime").textContent = formatDurationSeconds(uptimeSeconds);
    $("opsGomaxprocs").textContent = formatCount(gomaxprocs);

    const summaryLines = [
      `Sampled: ${new Date(sampledAt).toLocaleTimeString()}`,
      `CPU: ${formatPercent(cpuUsagePercent)} across ${formatCount(gomaxprocs)} Go scheduler slots`,
      `Memory: ${formatBytes(heapBytes)} heap / ${formatBytes(totalMemoryBytes)} total runtime`,
      `Concurrency: ${formatCount(goroutines)} goroutines / ${formatCount(threads)} threads`,
      `Requests served: ${formatCount(requests)} | GC cycles: ${formatCount(gcCycles)}`,
      `Uptime: ${formatDurationSeconds(uptimeSeconds)} | CPU time: ${formatMetricSeconds(cpuSeconds)}`,
    ];
    $("opsSummary").textContent = summaryLines.join("\n");
  }

  function stopOpsRefresh() {
    if (!state.opsRefreshTimer) return;
    clearInterval(state.opsRefreshTimer);
    state.opsRefreshTimer = null;
  }

  async function refreshRuntimeMetrics(options = {}) {
    const { silent = false, logPreview = false, showToastOnSuccess = false } = options;
    if (!requireBaseURL({ silent })) {
      return null;
    }

    let res;
    try {
      res = await fetch(`${state.baseUrl}/metrics`);
    } catch (err) {
      if (!silent) {
        logEvent("HTTP metrics", { method: "GET", path: "/metrics", error: String(err) }, "error");
        showToast("Network error. Check server URL and availability.", "error");
      }
      return null;
    }

    const text = await res.text();
    const { lineCount, preview } = metricsPreview(text);
    if (logPreview || !res.ok) {
      logEvent(
        "HTTP metrics",
        { method: "GET", path: "/metrics", status: res.status, line_count: lineCount, preview },
        res.ok ? "success" : "error",
      );
    }

    if (!res.ok) {
      if (!silent) {
        showToast(`HTTP metrics failed with status ${res.status}.`, "error");
      }
      return null;
    }

    renderRuntimeMetrics(parsePrometheusMetrics(text));
    if (showToastOnSuccess) {
      showToast("Runtime metrics refreshed.", "success");
    }

    return text;
  }

  function syncOpsRefresh() {
    stopOpsRefresh();
    if (state.activeTab !== "ops") return;

    void refreshRuntimeMetrics({ silent: true });
    state.opsRefreshTimer = window.setInterval(() => {
      void refreshRuntimeMetrics({ silent: true });
    }, 5000);
  }

  function showToast(message, type = "info") {
    const toast = $("toast");
    if (!toast) return;
    toast.textContent = message;
    toast.className = `toast ${type} show`;

    if (state.toastTimer) {
      clearTimeout(state.toastTimer);
    }
    state.toastTimer = setTimeout(() => {
      toast.className = "toast";
    }, 2600);
  }

  function renderLogs() {
    const box = $("events");
    if (!box) return;

    const counts = { all: state.logs.length, info: 0, success: 0, warn: 0, error: 0 };
    for (const entry of state.logs) {
      if (Object.prototype.hasOwnProperty.call(counts, entry.level)) {
        counts[entry.level] += 1;
      }
    }
    $("logAllCount").textContent = String(counts.all);
    $("logInfoCount").textContent = String(counts.info);
    $("logSuccessCount").textContent = String(counts.success);
    $("logWarnCount").textContent = String(counts.warn);
    $("logErrorCount").textContent = String(counts.error);
    for (const btn of document.querySelectorAll(".log-stat")) {
      const active = btn.dataset.level === state.logLevel;
      btn.classList.toggle("active", active);
      btn.setAttribute("aria-pressed", active ? "true" : "false");
    }

    const level = state.logLevel;
    const search = state.logSearch.trim().toLowerCase();
    const filtered = state.logs.filter((entry) => {
      const levelMatch = level === "all" || entry.level === level;
      if (!levelMatch) return false;
      if (!search) return true;
      const haystack = `${entry.kind}\n${safeJSON(entry.data)}`.toLowerCase();
      return haystack.includes(search);
    });

    box.innerHTML = "";
    if (filtered.length === 0) {
      const empty = document.createElement("div");
      empty.className = "empty";
      empty.textContent = state.logs.length === 0 ? "No events yet. Fill the common context, then run any room, message, push, load, or ops action." : "No logs match the current filter or search.";
      box.appendChild(empty);
    } else {
      for (const entry of filtered) {
        const wrap = document.createElement("article");
        wrap.className = `entry entry-${entry.level}`;

        const meta = document.createElement("div");
        meta.className = "entry-meta";

        const timestamp = document.createElement("span");
        timestamp.textContent = entry.timestamp;

        const badge = document.createElement("span");
        badge.className = `entry-level level-${entry.level}`;
        badge.textContent = entry.level;

        meta.append(timestamp, badge);

        const kind = document.createElement("div");
        kind.className = "entry-kind";
        kind.textContent = entry.kind;

        const body = document.createElement("pre");
        body.className = "entry-body";
        body.textContent = safeJSON(entry.data);

        wrap.append(meta, kind, body);
        box.appendChild(wrap);
      }
    }

    $("logCount").textContent = `${filtered.length} shown / ${state.logs.length} total`;

    if (state.autoScroll) {
      box.scrollTop = box.scrollHeight;
    }
  }

  function logEvent(kind, data, level = "info") {
    state.logs.push({
      id: state.nextLogID,
      timestamp: nowISO(),
      kind,
      data,
      level,
    });
    state.nextLogID += 1;
    renderLogs();
  }

  function normalizeBase(url) {
    const trimmed = (url || "").trim();
    if (!trimmed) return "";
    return trimmed.replace(/\/$/, "");
  }

  function isValidBaseURL(url) {
    try {
      const parsed = new URL(url);
      return parsed.protocol === "http:" || parsed.protocol === "https:";
    } catch {
      return false;
    }
  }

  function safeWSURL(base, userId, deviceId) {
    try {
      const u = new URL(base);
      u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
      u.pathname = "/v1/ws";
      u.search = userId && deviceId ? `?user_id=${encodeURIComponent(userId)}&device_id=${encodeURIComponent(deviceId)}` : "";
      return u.toString();
    } catch {
      return "";
    }
  }

  function genID(prefix) {
    return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1000)}`;
  }

  function wsURL(base) {
    return safeWSURL(base, state.userId, state.deviceId);
  }

  function setInvalid(el, isInvalid) {
    if (!el) return;
    el.classList.toggle("invalid", Boolean(isInvalid));
  }

  function clearCommonInvalid() {
    setInvalid($("baseUrl"), false);
    setInvalid($("userId"), false);
    setInvalid($("deviceId"), false);
    setInvalid($("roomId"), false);
  }

  function renderContextSummary() {
    const baseUrl = normalizeBase($("baseUrl") ? $("baseUrl").value : state.baseUrl);
    const userId = $("userId") ? $("userId").value.trim() : state.userId;
    const deviceId = $("deviceId") ? $("deviceId").value.trim() : state.deviceId;
    const roomId = $("roomId") ? $("roomId").value.trim() : "";
    const missing = [];
    let baseValid = false;

    if (!baseUrl) {
      missing.push("base URL");
    } else {
      baseValid = isValidBaseURL(baseUrl);
      if (!baseValid) {
        missing.push("valid base URL");
      }
    }
    if (!userId) missing.push("user ID");
    if (!deviceId) missing.push("device ID");

    let cardClass = "context-card";
    let stateLabel = "Waiting for context";
    let stateDetail = "Add a base URL, user ID, and device ID to unlock every action.";
    let headerSummary = "Fill the common context to unlock REST and WebSocket flows.";

    if (missing.length === 0 && state.wsConnected) {
      cardClass += " live";
      stateLabel = "Realtime session live";
      stateDetail = "REST and realtime actions are ready from the current helper session.";
      headerSummary = "Session is ready and WebSocket is connected.";
    } else if (missing.length === 0 && state.wsConnecting) {
      cardClass += " ready";
      stateLabel = "Connecting to realtime";
      stateDetail = "HTTP actions are ready. Waiting for the WebSocket handshake to finish.";
      headerSummary = "Common context is ready. Waiting for the WebSocket connection.";
    } else if (missing.length === 0) {
      cardClass += " ready";
      stateLabel = "REST ready";
      stateDetail = "HTTP flows are ready now. Connect WS when you want realtime send, sync, and ack.";
      headerSummary = "Common context is ready. Connect WS for realtime flows.";
    } else {
      stateDetail = `Missing ${missing.join(", ")}.`;
      headerSummary = `Fill ${missing.join(", ")} to unlock every flow.`;
    }

    $("contextCard").className = cardClass;
    $("contextState").textContent = stateLabel;
    $("contextStateDetail").textContent = stateDetail;
    $("headerSummary").textContent = headerSummary;
    $("contextHttpBase").textContent = baseUrl || "Not set";
    $("contextWsUrl").textContent = missing.length === 0 && baseValid ? safeWSURL(baseUrl, userId, deviceId) : "Complete required fields first";
    $("contextRoom").textContent = roomId || "No room selected";
    $("contextSocket").textContent = state.wsConnected ? "Connected" : state.wsConnecting ? "Connecting" : "Disconnected";
    $("btnCopyWsUrl").disabled = missing.length > 0 || !baseValid;
  }

  function requireBaseURL(options = {}) {
    const { silent = false } = options;
    setInvalid($("baseUrl"), false);

    state.baseUrl = normalizeBase($("baseUrl").value);
    renderContextSummary();

    if (state.baseUrl && isValidBaseURL(state.baseUrl)) {
      return true;
    }

    setInvalid($("baseUrl"), true);
    if (!silent) {
      logEvent("UI validation", "a valid base URL is required", "warn");
      showToast("Set a valid Base URL first.", "warn");
    }
    return false;
  }

  function refreshSessionUI() {
    $("sessionWs").textContent = state.wsConnected ? "connected" : state.wsConnecting ? "connecting" : "disconnected";
    $("wsDot").classList.toggle("on", state.wsConnected);

    const connectBusy = state.busy.has("btnConnect");
    const disconnectBusy = state.busy.has("btnDisconnect");

    $("btnConnect").disabled = state.wsConnected || state.wsConnecting || connectBusy;
    $("btnDisconnect").disabled = (!state.wsConnected && !state.wsConnecting) || disconnectBusy;

    const wsActionDisabled = !state.wsConnected;
    $("btnSendWs").disabled = wsActionDisabled;
    $("btnSync").disabled = wsActionDisabled;
    $("btnAck").disabled = wsActionDisabled;
    renderContextSummary();
  }

  function setBusy(id, busy) {
    const btn = $(id);
    if (!btn) return;

    if (busy) {
      state.busy.add(id);
      btn.classList.add("busy");
      btn.disabled = true;
    } else {
      state.busy.delete(id);
      btn.classList.remove("busy");
      btn.disabled = false;
    }

    refreshSessionUI();
  }

  async function runBusy(id, fn) {
    if (state.busy.has(id)) return;
    setBusy(id, true);
    try {
      await fn();
    } finally {
      setBusy(id, false);
    }
  }

  function saveSession() {
    localStorage.setItem(
      "aelora_poc_session",
      JSON.stringify({
        baseUrl: state.baseUrl,
        userId: state.userId,
        deviceId: state.deviceId,
        roomId: $("roomId").value.trim(),
      }),
    );
  }

  function loadSession() {
    const raw = localStorage.getItem("aelora_poc_session");
    if (!raw) {
      state.baseUrl = fallbackBaseURL();
      return;
    }

    try {
      const data = JSON.parse(raw);
      state.baseUrl = normalizeBase(data.baseUrl || fallbackBaseURL());
      state.userId = data.userId || "";
      state.deviceId = data.deviceId || "";
      if (data.roomId) {
        $("roomId").value = data.roomId;
      }
    } catch {
      state.baseUrl = fallbackBaseURL();
    }
  }

  function requireCommon() {
    clearCommonInvalid();

    const baseValid = requireBaseURL({ silent: true });
    state.userId = $("userId").value.trim();
    state.deviceId = $("deviceId").value.trim();
    renderContextSummary();

    let valid = baseValid;

    if (!state.userId) {
      setInvalid($("userId"), true);
      valid = false;
    }

    if (!state.deviceId) {
      setInvalid($("deviceId"), true);
      valid = false;
    }

    if (!valid) {
      logEvent("UI validation", "base URL, user ID and device ID are required and URL must be valid", "warn");
      showToast("Fill common fields with a valid Base URL, User ID, and Device ID.", "warn");
      return false;
    }

    return true;
  }

  function roomID() {
    return $("roomId").value.trim();
  }

  function ensureRoomID(context) {
    const rid = roomID();
    if (!rid) {
      setInvalid($("roomId"), true);
      logEvent("UI validation", `room ID is required for ${context}`, "warn");
      showToast("Room ID is required for this action.", "warn");
      return "";
    }
    setInvalid($("roomId"), false);
    return rid;
  }

  function parseMembers(input) {
    return (input || "")
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);
  }

  function userHeaders() {
    return { "X-User-ID": state.userId };
  }

  function jsonHeaders() {
    return { ...userHeaders(), "Content-Type": "application/json" };
  }

  async function callHTTP(path, options = {}, kind = "HTTP") {
    const { requirement = "common", ...requestOptions } = options;
    if (requirement === "base") {
      if (!requireBaseURL()) return null;
    } else if (!requireCommon()) {
      return null;
    }

    let res;
    try {
      res = await fetch(`${state.baseUrl}${path}`, requestOptions);
    } catch (err) {
      logEvent(kind, { method: requestOptions.method || "GET", path, error: String(err) }, "error");
      showToast("Network error. Check server URL and availability.", "error");
      return null;
    }

    let payload;
    const text = await res.text();
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }

    if (path === "/metrics" && typeof payload === "string") {
      payload = payload.split("\n").slice(0, 40).join("\n");
    }

    const level = res.ok ? "success" : "error";
    logEvent(kind, { method: requestOptions.method || "GET", path, status: res.status, payload }, level);

    if (!res.ok) {
      showToast(`${kind} failed with status ${res.status}.`, "error");
    }

    return { res, payload };
  }

  function scheduleLoadRender() {
    if (state.loadRenderQueued) return;
    state.loadRenderQueued = true;
    requestAnimationFrame(() => {
      state.loadRenderQueued = false;
      renderLoadTest();
    });
  }

  function startLoadTicker() {
    stopLoadTicker();
    state.loadTickTimer = window.setInterval(() => {
      scheduleLoadRender();
    }, 200);
  }

  function stopLoadTicker() {
    if (!state.loadTickTimer) return;
    clearInterval(state.loadTickTimer);
    state.loadTickTimer = null;
  }

  function loadRunState(load) {
    if (load.running && load.stopRequested) return "stopping";
    if (load.running) return "running";
    if (load.stopRequested && load.completed < load.total) return "stopped";
    if (load.finishedAt || load.completed > 0) return "finished";
    return "idle";
  }

  function loadSummaryText(load) {
    if (!load.config) {
      return "Load tester is idle. Configure a run and press Start.";
    }

    const latencyLabel = load.mode === "ws" ? "WS server ack latency" : "HTTP roundtrip latency";
    const elapsedMs = load.startedAt ? (load.finishedAt || performance.now()) - load.startedAt : 0;
    const successRate = load.completed > 0 ? `${Math.round((load.success / load.completed) * 100)}%` : "0%";
    const throughput = elapsedMs > 0 ? `${(load.completed / (elapsedMs / 1000)).toFixed(1)} req/s` : "0 req/s";
    const avgLatency = formatMilliseconds(average(load.latencies));
    const p95Latency = formatMilliseconds(percentile(load.latencies, 95));
    const p99Latency = formatMilliseconds(percentile(load.latencies, 99));
    const lines = [
      `Transport: ${loadTransportLabel(load.mode)}`,
      `Scenario: ${load.mode === "ws" ? "WS send_message -> message_accepted" : `POST /v1/rooms/${load.roomId}/messages`}`,
      `State: ${loadRunState(load)}`,
      `Total requests: ${load.total}`,
      `Completed: ${load.completed}`,
      `Success: ${load.success}`,
      `Failed: ${load.failed}`,
      `Concurrency: ${load.config.concurrency}`,
      `Worker delay: ${load.config.delayMs} ms`,
      `Payload size: ${load.config.payloadBytes} chars`,
      `Success rate: ${successRate}`,
      `Throughput: ${throughput}`,
      `Latency measured: ${latencyLabel}`,
      `Average latency: ${avgLatency}`,
      `P95 latency: ${p95Latency}`,
      `P99 latency: ${p99Latency}`,
      `Elapsed: ${formatMilliseconds(elapsedMs)}`,
    ];

    if (load.mode === "ws") {
      lines.push(`Ack timeout: ${load.config.ackTimeoutMs} ms`);
    }

    if (load.lastError) {
      lines.push(`Last error: ${load.lastError}`);
    }

    return lines.join("\n");
  }

  function updateLoadTransportHint() {
    const mode = $("loadMode") ? $("loadMode").value : "http";
    const hint = $("loadTransportHint");
    if (!hint) return;
    hint.textContent = mode === "ws" ? "WebSocket: send_message -> message_accepted (ack timeout active)" : "HTTP: POST /v1/rooms/{room_id}/messages (ack timeout ignored)";
  }

  function renderLoadTest() {
    if (!$("loadState")) return;

    const load = state.loadTest;
    const elapsedMs = load.startedAt ? (load.finishedAt || performance.now()) - load.startedAt : 0;
    const successRate = load.completed > 0 ? `${Math.round((load.success / load.completed) * 100)}%` : "0%";
    const throughput = elapsedMs > 0 ? `${(load.completed / (elapsedMs / 1000)).toFixed(1)} req/s` : "0 req/s";
    $("loadState").textContent = loadRunState(load);
    $("loadProgress").textContent = `${load.completed} / ${load.total}`;
    $("loadSuccess").textContent = String(load.success);
    $("loadFailure").textContent = String(load.failed);
    $("loadInFlight").textContent = String(load.inFlight);
    $("loadAvg").textContent = formatMilliseconds(average(load.latencies));
    $("loadP95").textContent = formatMilliseconds(percentile(load.latencies, 95));
    $("loadP99").textContent = formatMilliseconds(percentile(load.latencies, 99));
    $("loadElapsed").textContent = formatMilliseconds(elapsedMs);
    $("loadSuccessRate").textContent = successRate;
    $("loadThroughput").textContent = throughput;
    $("loadSummary").textContent = loadSummaryText(load);
    $("loadLastError").textContent = load.lastError || "none";
    $("btnStartLoad").disabled = load.running;
    $("btnStopLoad").disabled = !load.running;
    for (const id of ["loadMode", "loadTotal", "loadConcurrency", "loadDelayMs", "loadPayloadBytes", "loadPrefix"]) {
      const el = $(id);
      if (el) {
        el.disabled = load.running;
      }
    }
    const ackRelevant = $("loadMode").value === "ws";
    $("loadAckTimeoutField").classList.toggle("field-disabled", !ackRelevant);
    $("loadAckTimeoutMs").disabled = load.running || !ackRelevant;
    updateLoadTransportHint();
  }

  function readLoadConfig() {
    const modeEl = $("loadMode");
    const totalEl = $("loadTotal");
    const concurrencyEl = $("loadConcurrency");
    const delayEl = $("loadDelayMs");
    const payloadEl = $("loadPayloadBytes");
    const ackTimeoutEl = $("loadAckTimeoutMs");
    const prefixEl = $("loadPrefix");

    const mode = modeEl.value;
    const total = Number(totalEl.value || 0);
    const concurrency = Number(concurrencyEl.value || 0);
    const delayMs = Number(delayEl.value || 0);
    const payloadBytes = Number(payloadEl.value || 0);
    const ackTimeoutMs = Number(ackTimeoutEl.value || 0);
    const prefix = prefixEl.value.trim() || "load-test";

    let valid = true;
    const markInteger = (el, value, min, max) => {
      const ok = Number.isInteger(value) && value >= min && value <= max;
      setInvalid(el, !ok);
      valid = valid && ok;
    };

    markInteger(totalEl, total, 1, 10000);
    markInteger(concurrencyEl, concurrency, 1, 200);
    markInteger(delayEl, delayMs, 0, 10000);
    markInteger(payloadEl, payloadBytes, 8, 4000);
    if (mode === "ws") {
      markInteger(ackTimeoutEl, ackTimeoutMs, 100, 60000);
    } else {
      setInvalid(ackTimeoutEl, false);
    }
    setInvalid(prefixEl, !prefix);
    valid = valid && Boolean(prefix);
    setInvalid(modeEl, mode !== "http" && mode !== "ws");
    valid = valid && (mode === "http" || mode === "ws");

    if (!valid) {
      logEvent("Load test validation", "fix transport, total, concurrency, delay, payload size, ack timeout, and prefix before starting", "warn");
      showToast("Fix load-test settings before starting.", "warn");
      return null;
    }

    return { mode, total, concurrency, delayMs, payloadBytes, ackTimeoutMs: mode === "ws" ? ackTimeoutMs : 10000, prefix };
  }

  function buildLoadPayload(prefix, payloadBytes, index) {
    const label = `${prefix} #${index + 1}`;
    if (payloadBytes <= label.length) {
      return label.slice(0, payloadBytes);
    }

    const fillerLength = Math.max(0, payloadBytes - label.length - 1);
    return `${label} ${"x".repeat(fillerLength)}`;
  }

  function sleep(ms) {
    return new Promise((resolve) => {
      window.setTimeout(resolve, ms);
    });
  }

  function recordLoadSuccess(startedAt) {
    const load = state.loadTest;
    load.latencies.push(performance.now() - startedAt);
    load.completed += 1;
    load.success += 1;
  }

  function recordLoadFailure(index, message, detail, startedAt = 0) {
    const load = state.loadTest;
    if (startedAt > 0) {
      load.latencies.push(performance.now() - startedAt);
    }
    load.completed += 1;
    load.failed += 1;
    load.lastError = message;
    logEvent("Load test request failed", detail, "error");
  }

  function finishLoadAttempt() {
    const load = state.loadTest;
    load.inFlight = Math.max(0, load.inFlight - 1);

    if (load.completed > 0 && (load.completed % 25 === 0 || load.completed === load.total)) {
      logEvent("Load test progress", { completed: load.completed, total: load.total, success: load.success, failed: load.failed }, "info");
    }

    scheduleLoadRender();
  }

  async function executeHTTPLoadRequest(index, roomId, config) {
    const load = state.loadTest;
    load.inFlight += 1;
    scheduleLoadRender();

    const started = performance.now();
    const clientMsgID = `${config.prefix}-${Date.now()}-${index}-${Math.floor(Math.random() * 10000)}`;
    const content = buildLoadPayload(config.prefix, config.payloadBytes, index);

    try {
      const res = await fetch(`${state.baseUrl}/v1/rooms/${encodeURIComponent(roomId)}/messages`, {
        method: "POST",
        headers: jsonHeaders(),
        body: JSON.stringify({
          content,
          client_msg_id: clientMsgID,
        }),
      });

      let failure = null;
      if (res.ok) {
        await res.text();
      } else {
        const text = await res.text();
        let payload = text;
        try {
          payload = JSON.parse(text);
        } catch {
          // keep text
        }
        failure = { status: res.status, payload };
      }

      if (failure) {
        recordLoadFailure(index, `request #${index + 1} returned HTTP ${failure.status}`, { request: index + 1, status: failure.status, payload: failure.payload }, started);
      } else {
        recordLoadSuccess(started);
      }
    } catch (err) {
      recordLoadFailure(index, `request #${index + 1} failed: ${String(err)}`, { request: index + 1, error: String(err) }, started);
    } finally {
      finishLoadAttempt();
    }
  }

  async function runHTTPLoadWorker(roomId, config) {
    while (true) {
      if (state.loadTest.stopRequested) return;

      const index = state.loadTest.nextIndex;
      if (index >= config.total) return;
      state.loadTest.nextIndex += 1;

      await executeHTTPLoadRequest(index, roomId, config);

      if (config.delayMs > 0 && !state.loadTest.stopRequested) {
        await sleep(config.delayMs);
      }
    }
  }

  function clearWSPendingAck(clientMsgID) {
    const pending = state.loadTest.wsPending.get(clientMsgID);
    if (!pending) return null;
    clearTimeout(pending.timeoutId);
    state.loadTest.wsPending.delete(clientMsgID);
    return pending;
  }

  function handleWSLoadAck(clientMsgID) {
    const pending = clearWSPendingAck(clientMsgID);
    if (!pending) return false;

    recordLoadSuccess(pending.startedAt);
    finishLoadAttempt();
    return true;
  }

  function failOutstandingWSLoad(reason) {
    const pendingEntries = Array.from(state.loadTest.wsPending.entries());
    for (const [clientMsgID, pending] of pendingEntries) {
      clearTimeout(pending.timeoutId);
      state.loadTest.wsPending.delete(clientMsgID);
      recordLoadFailure(pending.index, `${clientMsgID}: ${reason}`, { client_msg_id: clientMsgID, error: reason }, pending.startedAt);
      finishLoadAttempt();
    }
  }

  function sendWSFrame(frame, options = {}) {
    const { log = true, errorKind = "WS send error" } = options;
    if (!state.wsConnected || !state.ws) {
      if (log) {
        logEvent("WS send", "not connected", "warn");
        showToast("Connect WebSocket first.", "warn");
      }
      return false;
    }

    try {
      state.ws.send(JSON.stringify(frame));
      if (log) {
        logEvent("WS send", frame, "success");
      }
      return true;
    } catch (err) {
      if (log) {
        logEvent(errorKind, String(err), "error");
        showToast("Failed to send websocket frame.", "error");
      }
      return false;
    }
  }

  function startWSPendingAck(index, clientMsgID, ackTimeoutMs) {
    const startedAt = performance.now();
    const timeoutId = window.setTimeout(() => {
      const pending = clearWSPendingAck(clientMsgID);
      if (!pending) return;
      recordLoadFailure(index, `${clientMsgID}: ack timeout after ${ackTimeoutMs} ms`, { client_msg_id: clientMsgID, error: "ack timeout", timeout_ms: ackTimeoutMs }, pending.startedAt);
      finishLoadAttempt();
    }, ackTimeoutMs);

    state.loadTest.wsPending.set(clientMsgID, { startedAt, timeoutId, index });
    return startedAt;
  }

  function dispatchWSLoadMessage(index, roomId, config) {
    const load = state.loadTest;
    load.inFlight += 1;
    scheduleLoadRender();

    const clientMsgID = `${config.prefix}-${Date.now()}-${index}-${Math.floor(Math.random() * 10000)}`;
    const content = buildLoadPayload(config.prefix, config.payloadBytes, index);
    const startedAt = startWSPendingAck(index, clientMsgID, config.ackTimeoutMs);
    const frame = {
      type: "send_message",
      room_id: roomId,
      content,
      client_msg_id: clientMsgID,
      reply_to_message_id: "",
    };

    if (!sendWSFrame(frame, { log: false, errorKind: "WS load send error" })) {
      clearWSPendingAck(clientMsgID);
      recordLoadFailure(index, `${clientMsgID}: websocket send failed`, { client_msg_id: clientMsgID, error: "websocket send failed" }, startedAt);
      finishLoadAttempt();
      return false;
    }
    return true;
  }

  async function runWSLoadTest(roomId, config) {
    while (true) {
      const load = state.loadTest;
      if (load.stopRequested && load.inFlight === 0) return;

      while (!load.stopRequested && load.nextIndex < config.total && load.inFlight < config.concurrency) {
        const index = load.nextIndex;
        load.nextIndex += 1;
        dispatchWSLoadMessage(index, roomId, config);
        if (config.delayMs > 0) {
          await sleep(config.delayMs);
        }
      }

      if (load.nextIndex >= config.total && load.inFlight === 0) {
        return;
      }

      await sleep(10);
    }
  }

  async function runLoadTest() {
    if (state.loadTest.running) return;
    if (!requireCommon()) return;

    const roomId = ensureRoomID("load test");
    if (!roomId) return;

    const config = readLoadConfig();
    if (!config) return;
    if (config.mode === "ws" && (!state.wsConnected || !state.ws)) {
      logEvent("Load test validation", "connect WebSocket before starting WS load mode", "warn");
      showToast("Connect WebSocket before starting WS load mode.", "warn");
      return;
    }

    saveSession();
    state.loadTest = {
      ...newLoadTestState(),
      running: true,
      mode: config.mode,
      total: config.total,
      startedAt: performance.now(),
      roomId,
      config,
    };
    startLoadTicker();
    scheduleLoadRender();

    logEvent("Load test started", {
      mode: config.mode,
      room_id: roomId,
      total: config.total,
      concurrency: config.concurrency,
      delay_ms: config.delayMs,
      payload_bytes: config.payloadBytes,
      ack_timeout_ms: config.ackTimeoutMs,
    }, "info");
    showToast(`${loadTransportLabel(config.mode)} load test started with ${config.total} requests.`, "info");

    try {
      if (config.mode === "ws") {
        await runWSLoadTest(roomId, config);
      } else {
        const workerCount = Math.min(config.total, config.concurrency);
        const workers = [];
        for (let i = 0; i < workerCount; i += 1) {
          workers.push(runHTTPLoadWorker(roomId, config));
        }
        await Promise.all(workers);
      }
    } finally {
      const load = state.loadTest;
      load.running = false;
      load.finishedAt = performance.now();
      stopLoadTicker();
      renderLoadTest();

      const stopped = load.stopRequested && load.completed < load.total;
      const avgMs = Number(average(load.latencies).toFixed(1));
      const p95Ms = Number(percentile(load.latencies, 95).toFixed(1));
      const p99Ms = Number(percentile(load.latencies, 99).toFixed(1));
      logEvent(
        "Load test finished",
        {
          mode: load.mode,
          room_id: roomId,
          total: load.total,
          completed: load.completed,
          success: load.success,
          failed: load.failed,
          stopped,
          avg_ms: avgMs,
          p95_ms: p95Ms,
          p99_ms: p99Ms,
          elapsed_ms: Math.round(load.finishedAt - load.startedAt),
        },
        load.failed > 0 || stopped ? "warn" : "success",
      );
      showToast(
        stopped ? `${loadTransportLabel(load.mode)} load test stopped at ${load.completed}/${load.total}.` : `${loadTransportLabel(load.mode)} load test finished: ${load.success} succeeded.`,
        load.failed > 0 || stopped ? "warn" : "success",
      );
    }
  }

  function stopLoadTest() {
    if (!state.loadTest.running) {
      showToast("No load test is running.", "warn");
      return;
    }
    if (state.loadTest.stopRequested) return;

    state.loadTest.stopRequested = true;
    scheduleLoadRender();
    logEvent("Load test stopping", { completed: state.loadTest.completed, in_flight: state.loadTest.inFlight }, "warn");
    showToast("Stopping after in-flight requests finish.", "warn");
  }

  async function createRoom() {
    if (!requireCommon()) return;

    const members = parseMembers($("createMembers").value).filter((id) => id !== state.userId);
    const out = await callHTTP(
      "/v1/rooms",
      {
        method: "POST",
        headers: jsonHeaders(),
        body: JSON.stringify({ member_ids: members }),
      },
      "HTTP create room",
    );

    const roomID = out?.payload?.data?.room_id || out?.payload?.room_id || out?.payload?.id || "";
    if (out && out.res.ok && roomID) {
      $("roomId").value = roomID;
      saveSession();
      renderContextSummary();
      showToast(`Room created: ${roomID}`, "success");
    }
  }

  async function joinRoom() {
    if (!requireCommon()) return;
    const rid = ensureRoomID("join");
    const target = $("joinUser").value.trim();

    if (!rid || !target) {
      if (!target) setInvalid($("joinUser"), true);
      logEvent("UI validation", "room ID and target user are required", "warn");
      showToast("Room ID and target user are required.", "warn");
      return;
    }
    setInvalid($("joinUser"), false);

    const out = await callHTTP(
      `/v1/rooms/${encodeURIComponent(rid)}/members/${encodeURIComponent(target)}`,
      { method: "PUT", headers: userHeaders() },
      "HTTP join member",
    );

    if (out && out.res.ok) {
      showToast(`Member ${target} added to ${rid}.`, "success");
    }
  }

  async function sendHTTPMessage() {
    if (!requireCommon()) return;
    const rid = ensureRoomID("HTTP message send");
    const content = $("httpContent").value.trim();
    const clientMsgID = $("httpClientMsgId").value.trim();
    const replyToID = $("httpReplyToId").value.trim();

    if (!rid || !content || !clientMsgID) {
      setInvalid($("httpContent"), !content);
      setInvalid($("httpClientMsgId"), !clientMsgID);
      logEvent("UI validation", "room ID, content and client message ID are required", "warn");
      showToast("Room ID, content and client message ID are required.", "warn");
      return;
    }

    setInvalid($("httpContent"), false);
    setInvalid($("httpClientMsgId"), false);

    const out = await callHTTP(
      `/v1/rooms/${encodeURIComponent(rid)}/messages`,
      {
        method: "POST",
        headers: jsonHeaders(),
        body: JSON.stringify({
          content,
          client_msg_id: clientMsgID,
          reply_to_message_id: replyToID,
        }),
      },
      "HTTP send message",
    );

    if (out && out.res.ok) {
      showToast("HTTP message sent.", "success");
      if (!replyToID) {
        $("httpReplyToId").value = "";
      }
      $("httpContent").value = "";
    }
  }

  async function listMessages() {
    if (!requireCommon()) return;
    const rid = ensureRoomID("list messages");
    if (!rid) return;

    const limit = Number($("listLimit").value || 50);
    const since = $("listSince").value.trim();
    const query = new URLSearchParams();
    if (limit > 0) query.set("limit", String(limit));
    if (since) query.set("since", since);
    const qs = query.toString();

    const out = await callHTTP(
      `/v1/rooms/${encodeURIComponent(rid)}/messages${qs ? `?${qs}` : ""}`,
      { method: "GET", headers: userHeaders() },
      "HTTP list messages",
    );

    if (out && out.res.ok) {
      showToast("Room messages loaded.", "success");
    }
  }

  async function registerPush() {
    if (!requireCommon()) return;

    const device = $("pushDeviceId").value.trim() || state.deviceId;
    const platform = $("pushPlatform").value;
    const token = $("pushToken").value.trim();

    if (!device || !token) {
      setInvalid($("pushDeviceId"), !device);
      setInvalid($("pushToken"), !token);
      logEvent("UI validation", "push device ID and token are required", "warn");
      showToast("Push device ID and token are required.", "warn");
      return;
    }

    setInvalid($("pushDeviceId"), false);
    setInvalid($("pushToken"), false);

    const out = await callHTTP(
      `/v1/users/me/devices/${encodeURIComponent(device)}/push-token`,
      {
        method: "PUT",
        headers: jsonHeaders(),
        body: JSON.stringify({ platform, token }),
      },
      "HTTP register push token",
    );

    if (out && out.res.ok) {
      showToast("Push token registered.", "success");
    }
  }

  async function deletePush() {
    if (!requireCommon()) return;

    const device = $("pushDeviceId").value.trim() || state.deviceId;
    if (!device) {
      setInvalid($("pushDeviceId"), true);
      logEvent("UI validation", "push device ID is required", "warn");
      showToast("Push device ID is required.", "warn");
      return;
    }

    setInvalid($("pushDeviceId"), false);

    const out = await callHTTP(
      `/v1/users/me/devices/${encodeURIComponent(device)}/push-token`,
      {
        method: "DELETE",
        headers: userHeaders(),
      },
      "HTTP delete push token",
    );

    if (out && out.res.ok) {
      showToast("Push token deleted.", "success");
    }
  }

  function connectWS() {
    if (!requireCommon()) return;
    if (state.wsConnected || state.wsConnecting) {
      showToast("WebSocket is already connected or connecting.", "warn");
      return;
    }

    state.wsConnecting = true;
    refreshSessionUI();

    const ws = new WebSocket(wsURL(state.baseUrl));

    ws.onopen = () => {
      state.ws = ws;
      state.wsConnected = true;
      state.wsConnecting = false;
      refreshSessionUI();
      logEvent("WS open", { user: state.userId, device: state.deviceId }, "success");
      showToast("WebSocket connected.", "success");
    };

    ws.onclose = (event) => {
      state.wsConnected = false;
      state.wsConnecting = false;
      state.ws = null;
      if (state.loadTest.running && state.loadTest.mode === "ws") {
        state.loadTest.stopRequested = true;
      }
      if (state.loadTest.running && state.loadTest.mode === "ws" && state.loadTest.wsPending.size > 0) {
        failOutstandingWSLoad("websocket disconnected");
      }
      refreshSessionUI();
      logEvent("WS close", { code: event.code, reason: event.reason }, "warn");
      showToast("WebSocket disconnected.", "warn");
    };

    ws.onerror = () => {
      logEvent("WS error", "socket error", "error");
      showToast("WebSocket error occurred.", "error");
    };

    ws.onmessage = (event) => {
      let payload = event.data;
      try {
        payload = JSON.parse(event.data);
      } catch {
        // keep raw
      }

      let handledLoadAck = false;
      if (payload && payload.type === "message_accepted") {
        const ackClientMsgID = payload.message && payload.message.client_msg_id;
        if (ackClientMsgID) {
          handledLoadAck = handleWSLoadAck(ackClientMsgID);
        }
      }

      if (!handledLoadAck) {
        logEvent("WS recv", payload, "info");
      }

      if (payload && payload.type === "error" && state.loadTest.running && state.loadTest.mode === "ws") {
        state.loadTest.lastError = payload.error || "websocket error frame";
        scheduleLoadRender();
      }

      if (payload && payload.type === "sync_result" && payload.message && Array.isArray(payload.message.messages) && payload.message.messages.length > 0) {
        const last = payload.message.messages[payload.message.messages.length - 1];
        if (last && last.stream_id) {
          $("ackStreamId").value = last.stream_id;
        }
      }
    };
  }

  function disconnectWS() {
    if (!state.ws && !state.wsConnecting) {
      showToast("WebSocket is not connected.", "warn");
      return;
    }

    if (state.ws) {
      state.ws.close();
    }
    state.ws = null;
    state.wsConnecting = false;
    state.wsConnected = false;
    refreshSessionUI();
  }

  function sendWS(type, payload) {
    const frame = { type, ...payload };
    return sendWSFrame(frame, { log: true, errorKind: "WS send error" });
  }

  function sendMessageWS() {
    const rid = ensureRoomID("WS send_message");
    const content = $("msgContent").value.trim();
    const clientMsgID = $("clientMsgId").value.trim();
    const replyToID = $("replyToId").value.trim();

    if (!rid || !content || !clientMsgID) {
      setInvalid($("msgContent"), !content);
      setInvalid($("clientMsgId"), !clientMsgID);
      logEvent("UI validation", "room ID, content and client message ID are required", "warn");
      showToast("Room ID, content and client message ID are required.", "warn");
      return;
    }

    setInvalid($("msgContent"), false);
    setInvalid($("clientMsgId"), false);

    const sent = sendWS("send_message", {
      room_id: rid,
      content,
      client_msg_id: clientMsgID,
      reply_to_message_id: replyToID,
    });

    if (sent) {
      showToast("WebSocket message sent.", "success");
      $("msgContent").value = "";
    }
  }

  function syncWS() {
    const rid = ensureRoomID("WS SYNC");
    if (!rid) return;
    const lastAck = $("lastAck").value.trim() || "0";
    const limit = Number($("syncLimit").value || 100);

    const sent = sendWS("SYNC", { room_id: rid, last_ack: lastAck, limit });
    if (sent) {
      showToast("SYNC request sent.", "success");
    }
  }

  function ackWS() {
    const rid = ensureRoomID("WS ACK");
    const streamID = $("ackStreamId").value.trim();

    if (!rid || !streamID) {
      setInvalid($("ackStreamId"), !streamID);
      logEvent("UI validation", "room ID and stream ID are required for ACK", "warn");
      showToast("Room ID and stream ID are required for ACK.", "warn");
      return;
    }

    setInvalid($("ackStreamId"), false);

    const sent = sendWS("ACK", { room_id: rid, stream_id: streamID });
    if (sent) {
      showToast("ACK sent.", "success");
    }
  }

  function exportLog() {
    const payload = {
      exported_at: nowISO(),
      total: state.logs.length,
      logs: state.logs,
    };

    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `aelora-console-log-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);

    showToast("Logs exported.", "success");
  }

  async function copyText(value, successMessage) {
    if (!value) {
      showToast("Nothing to copy yet.", "warn");
      return;
    }

    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        const textarea = document.createElement("textarea");
        textarea.value = value;
        textarea.setAttribute("readonly", "true");
        textarea.style.position = "absolute";
        textarea.style.left = "-9999px";
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand("copy");
        textarea.remove();
      }
      showToast(successMessage, "success");
    } catch (err) {
      logEvent("UI helper", { action: "copy", error: String(err) }, "warn");
      showToast("Copy failed in this browser.", "warn");
    }
  }

  function fillDemoValues() {
    const fallbackBase = fallbackBaseURL();

    if (!$("baseUrl").value.trim()) {
      $("baseUrl").value = fallbackBase;
    }
    if (!$("userId").value.trim()) {
      $("userId").value = `user_${Math.floor(Math.random() * 9000) + 1000}`;
    }
    if (!$("deviceId").value.trim()) {
      $("deviceId").value = `web_${Math.floor(Math.random() * 9000) + 1000}`;
    }

    $("pushDeviceId").value = $("deviceId").value.trim();
    renderContextSummary();

    showToast("Demo values applied.", "success");
    logEvent("UI helper", "demo values filled", "info");
  }

  function wireFieldHygiene() {
    const clearOnInput = [
      "baseUrl",
      "userId",
      "deviceId",
      "roomId",
      "joinUser",
      "httpContent",
      "httpClientMsgId",
      "msgContent",
      "clientMsgId",
      "pushDeviceId",
      "pushToken",
      "ackStreamId",
      "loadMode",
      "loadTotal",
      "loadConcurrency",
      "loadDelayMs",
      "loadPayloadBytes",
      "loadAckTimeoutMs",
      "loadPrefix",
    ];
    const contextFields = new Set(["baseUrl", "userId", "deviceId", "roomId"]);

    for (const id of clearOnInput) {
      const el = $(id);
      if (!el) continue;
      el.addEventListener("input", () => {
        setInvalid(el, false);
        if (contextFields.has(id)) {
          renderContextSummary();
        }
      });
    }

    $("deviceId").addEventListener("input", () => {
      const nextDeviceID = $("deviceId").value.trim();
      const currentPushDevice = $("pushDeviceId").value.trim();
      if (!currentPushDevice || currentPushDevice === state.deviceId) {
        $("pushDeviceId").value = nextDeviceID;
      }
      state.deviceId = nextDeviceID;
      renderContextSummary();
    });
  }

  function activateTab(tabName) {
    const tabButtons = document.querySelectorAll(".tab-btn");
    const tabPanels = document.querySelectorAll(".tab-panel");
    if (tabButtons.length === 0 || tabPanels.length === 0) return;

    state.activeTab = tabName;

    for (const btn of tabButtons) {
      const isActive = btn.dataset.tab === tabName;
      btn.classList.toggle("active", isActive);
      btn.setAttribute("aria-selected", isActive ? "true" : "false");
    }

    for (const panel of tabPanels) {
      const isActive = panel.dataset.tab === tabName;
      panel.classList.toggle("active", isActive);
    }

    localStorage.setItem("aelora_poc_active_tab", tabName);
    syncOpsRefresh();
  }

  function wireTabs() {
    const tabButtons = document.querySelectorAll(".tab-btn");
    if (tabButtons.length === 0) return;
    const validTabs = new Set(Array.from(tabButtons).map((btn) => btn.dataset.tab || ""));

    for (const btn of tabButtons) {
      btn.setAttribute("role", "tab");
      btn.setAttribute("aria-selected", btn.classList.contains("active") ? "true" : "false");
      btn.addEventListener("click", () => activateTab(btn.dataset.tab || "rooms"));
    }

    const saved = localStorage.getItem("aelora_poc_active_tab");
    if (saved && validTabs.has(saved)) {
      activateTab(saved);
    } else {
      const initial = document.querySelector(".tab-btn.active");
      activateTab(initial ? initial.dataset.tab || "rooms" : "rooms");
    }
  }

  function wire() {
    loadSession();

    $("baseUrl").value = state.baseUrl || fallbackBaseURL();
    $("userId").value = state.userId;
    $("deviceId").value = state.deviceId;
    $("pushDeviceId").value = state.deviceId;

    $("httpClientMsgId").value = genID("c-http");
    $("clientMsgId").value = genID("c-ws");

    refreshSessionUI();
    renderLogs();
    renderLoadTest();
    wireFieldHygiene();
    wireTabs();

    $("btnSaveSession").addEventListener("click", () => {
      if (!requireCommon()) return;
      saveSession();
      logEvent(
        "Session saved",
        {
          baseUrl: state.baseUrl,
          userId: state.userId,
          deviceId: state.deviceId,
          roomId: roomID(),
        },
        "success",
      );
      showToast("Session saved.", "success");
    });

    $("btnConnect").addEventListener("click", () => runBusy("btnConnect", async () => connectWS()));
    $("btnDisconnect").addEventListener("click", () => runBusy("btnDisconnect", async () => disconnectWS()));
    $("btnUseOrigin").addEventListener("click", () => {
      $("baseUrl").value = fallbackBaseURL();
      setInvalid($("baseUrl"), false);
      renderContextSummary();
      showToast("Base URL set to the current helper origin.", "success");
    });
    $("btnCopyWsUrl").addEventListener("click", async () => {
      const url = safeWSURL(normalizeBase($("baseUrl").value), $("userId").value.trim(), $("deviceId").value.trim());
      await copyText(url, "WebSocket URL copied.");
    });

    $("btnFillDemo").addEventListener("click", fillDemoValues);

    $("btnClearLog").addEventListener("click", () => {
      state.logs = [];
      state.nextLogID = 1;
      renderLogs();
      showToast("Logs cleared.", "success");
    });

    $("btnAutoScroll").addEventListener("click", () => {
      state.autoScroll = !state.autoScroll;
      $("btnAutoScroll").textContent = `Auto Scroll: ${state.autoScroll ? "On" : "Off"}`;
      if (state.autoScroll) {
        const box = $("events");
        box.scrollTop = box.scrollHeight;
      }
    });

    $("btnExportLog").addEventListener("click", exportLog);
    $("loadMode").addEventListener("change", () => {
      setInvalid($("loadMode"), false);
      updateLoadTransportHint();
      renderLoadTest();
    });

    for (const btn of document.querySelectorAll(".log-stat")) {
      btn.addEventListener("click", () => {
        state.logLevel = btn.dataset.level || "all";
        renderLogs();
      });
    }

    $("logSearch").addEventListener("input", () => {
      state.logSearch = $("logSearch").value;
      renderLogs();
    });

    $("btnCreateRoom").addEventListener("click", () => runBusy("btnCreateRoom", createRoom));
    $("btnJoin").addEventListener("click", () => runBusy("btnJoin", joinRoom));

    $("btnGenHttpClientId").addEventListener("click", () => {
      $("httpClientMsgId").value = genID("c-http");
      setInvalid($("httpClientMsgId"), false);
    });
    $("btnSendHttp").addEventListener("click", () => runBusy("btnSendHttp", sendHTTPMessage));
    $("btnListMessages").addEventListener("click", () => runBusy("btnListMessages", listMessages));

    $("btnGenClientId").addEventListener("click", () => {
      $("clientMsgId").value = genID("c-ws");
      setInvalid($("clientMsgId"), false);
    });
    $("btnSendWs").addEventListener("click", sendMessageWS);
    $("btnSync").addEventListener("click", syncWS);
    $("btnAck").addEventListener("click", ackWS);

    $("btnRegisterPush").addEventListener("click", () => runBusy("btnRegisterPush", registerPush));
    $("btnDeletePush").addEventListener("click", () => runBusy("btnDeletePush", deletePush));

    $("btnStartLoad").addEventListener("click", runLoadTest);
    $("btnStopLoad").addEventListener("click", stopLoadTest);

    $("btnLivez").addEventListener("click", () => runBusy("btnLivez", async () => callHTTP("/livez", { requirement: "base" }, "HTTP livez")));
    $("btnReadyz").addEventListener("click", () => runBusy("btnReadyz", async () => callHTTP("/readyz", { requirement: "base" }, "HTTP readyz")));
    $("btnHealthz").addEventListener("click", () => runBusy("btnHealthz", async () => callHTTP("/healthz", { requirement: "base" }, "HTTP healthz")));
    $("btnMetrics").addEventListener("click", () => runBusy("btnMetrics", async () => refreshRuntimeMetrics({ logPreview: true, showToastOnSuccess: true })));
    $("btnRefreshRuntime").addEventListener("click", () => runBusy("btnRefreshRuntime", async () => refreshRuntimeMetrics({ showToastOnSuccess: true })));

    logEvent("Console ready", "Common fields loaded. Save session, then call actions.", "info");
    renderContextSummary();
  }

  wire();
})();
