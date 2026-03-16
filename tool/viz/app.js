const state = {
  events: [],
  byTime: new Map(),
  timeKeys: [],
  minTime: 0,
  maxTime: 0,
  currentTime: 0,
  maxX: 0,
  maxY: 0,
  timer: null,
  speedMs: 1100,
  showDataFlow: true,
  showInst: true,
  showMemory: true,
  showLabels: true,
  programSpec: null,
  yamlGridBounds: null,
  reportSpec: null,
  reportReady: false,
  reportError: "",
  reportHeatMetric: "utilizationPct",
  timingRows: [],
  timingColumns: [],
  timingReady: false,
  layoutMode: "fit",
  timingAnomalyOnly: false,
  timingSelectedCell: null,
  timingFocusedCoreKey: null,
  showPhaseExplain: true,
  timingBoundaryOnly: false,
  timingBaselineView: "compensated",
  timingCompModel: "hybrid",
  timingIoWaveExpandAll: false,
  timingIoWaveExpandedCoreKeys: new Set(),
  timingWindowStart: 0,
  timingWindowSize: 120,
  timingZoomX: 1,
  timingZoomY: 1,
  timingViewport: null,
  firstHybridMismatchTime: null,
  coreIoWaveByTime: new Map(),
  stepLock: false,
};

const layout = {
  baseWidth: 940,
  baseHeight: 620,
  baseTileSize: 100,
  baseGap: 24,
  baseDriverOffset: 52,
  marginLeft: 170,
  marginRight: 92,
  marginTop: 90,
  marginBottom: 88,
  minTileSize: 28,
  maxTileSize: 124,
  minReadableTile: 36,
  minGap: 7,
  maxGap: 28,
  minDriverOffset: 20,
  maxDriverOffset: 66,
  width: 940,
  height: 620,
  originX: 170,
  originY: 90,
  tileSize: 100,
  gap: 24,
  driverOffset: 52,
};

const colors = {
  Send: "#006d77",
  Recv: "#118ab2",
  FeedIn: "#ef476f",
  Collect: "#8338ec",
  Inst: "#f77f00",
  Memory: "#d62828",
};

const svg = d3.select("#canvas");
let sceneRoot;
let staticLayer;
let dynamicLayer;
let meshZoomBehavior = null;
let meshZoomTransform = d3.zoomIdentity;

const controls = {
  playBtn: document.getElementById("playBtn"),
  stepBackBtn: document.getElementById("stepBackBtn"),
  stepFwdBtn: document.getElementById("stepFwdBtn"),
  speedSelect: document.getElementById("speedSelect"),
  timeSlider: document.getElementById("timeSlider"),
  timeLabel: document.getElementById("timeLabel"),
  showDataFlow: document.getElementById("showDataFlow"),
  showInst: document.getElementById("showInst"),
  showMemory: document.getElementById("showMemory"),
  showLabels: document.getElementById("showLabels"),
  fileInput: document.getElementById("fileInput"),
  yamlInput: document.getElementById("yamlInput"),
  reportInput: document.getElementById("reportInput"),
  statsLine: document.getElementById("statsLine"),
  eventDump: document.getElementById("eventDump"),
  reportSummary: document.getElementById("reportSummary"),
  reportHotTiles: document.getElementById("reportHotTiles"),
  reportWarning: document.getElementById("reportWarning"),
  timingSummary: document.getElementById("timingSummary"),
  timingGrid: document.getElementById("timingGrid"),
  timingAnomalyOnly: document.getElementById("timingAnomalyOnly"),
  timingShowPhaseExplain: document.getElementById("timingShowPhaseExplain"),
  timingBoundaryOnly: document.getElementById("timingBoundaryOnly"),
  timingCoreFocus: document.getElementById("timingCoreFocus"),
  timingIoWaveAll: document.getElementById("timingIoWaveAll"),
  timingIoWaveCore: document.getElementById("timingIoWaveCore"),
  timingBaselineView: document.getElementById("timingBaselineView"),
  timingCompModel: document.getElementById("timingCompModel"),
  timingWindowStart: document.getElementById("timingWindowStart"),
  timingWindowSize: document.getElementById("timingWindowSize"),
  timingWindowStartLabel: document.getElementById("timingWindowStartLabel"),
  timingWindowSizeLabel: document.getElementById("timingWindowSizeLabel"),
  timingZoomY: document.getElementById("timingZoomY"),
  timingZoomYLabel: document.getElementById("timingZoomYLabel"),
  timingResetZoom: document.getElementById("timingResetZoom"),
  timingExportPng: document.getElementById("timingExportPng"),
  timingExportMaxSide: document.getElementById("timingExportMaxSide"),
  timingJumpFirstMismatch: document.getElementById("timingJumpFirstMismatch"),
  timingDrilldown: document.getElementById("timingDrilldown"),
  timingCoreMini: document.getElementById("timingCoreMini"),
  meshLegend: document.getElementById("meshLegend"),
  vizPanel: document.querySelector(".panel.viz"),
};
let timingCoreLabelClickTimer = null;

function tileKey(x, y) {
  return `${x},${y}`;
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function normalizeCycleTime(value, fallback = 0) {
  const numeric = Math.round(Number(value));
  return Number.isFinite(numeric) ? numeric : fallback;
}

function nextIndexedTime(current, direction) {
  const dir = direction >= 0 ? 1 : -1;
  const keys = Array.isArray(state.timeKeys) ? state.timeKeys : [];
  const cur = normalizeCycleTime(current, state.minTime);
  if (keys.length === 0) {
    const target = cur + dir;
    return clamp(target, state.minTime, state.maxTime);
  }
  const exactIdx = keys.indexOf(cur);
  if (exactIdx >= 0) {
    const nextIdx = clamp(exactIdx + dir, 0, keys.length - 1);
    return keys[nextIdx];
  }
  if (dir > 0) {
    for (const t of keys) {
      if (t > cur) return t;
    }
    return keys[keys.length - 1];
  }
  for (let i = keys.length - 1; i >= 0; i -= 1) {
    if (keys[i] < cur) return keys[i];
  }
  return keys[0];
}

function resolveTargetViewport() {
  const hostWidth = controls.vizPanel?.clientWidth || layout.baseWidth;
  const width = Math.max(720, Math.round(hostWidth) - 8);
  const height = Math.max(480, Math.round(width * (layout.baseHeight / layout.baseWidth)));
  return { width, height };
}

function applyAdaptiveLayout() {
  const cols = Math.max(1, state.maxX + 1);
  const rows = Math.max(1, state.maxY + 1);
  const { width: targetWidth, height: targetHeight } = resolveTargetViewport();

  const contentW = Math.max(1, targetWidth - layout.marginLeft - layout.marginRight);
  const contentH = Math.max(1, targetHeight - layout.marginTop - layout.marginBottom);
  const baseGridW = cols * layout.baseTileSize + (cols - 1) * layout.baseGap;
  const baseGridH = rows * layout.baseTileSize + (rows - 1) * layout.baseGap;
  const fitScale = Math.min(contentW / baseGridW, contentH / baseGridH);
  const boundedScale = clamp(fitScale, 0.2, 1.45);

  let tileSize = clamp(
    Math.round(layout.baseTileSize * boundedScale),
    layout.minTileSize,
    layout.maxTileSize,
  );
  let gap = clamp(Math.round(layout.baseGap * boundedScale), layout.minGap, layout.maxGap);
  let driverOffset = clamp(
    Math.round(layout.baseDriverOffset * boundedScale),
    layout.minDriverOffset,
    layout.maxDriverOffset,
  );
  let mode = "fit";
  if (tileSize < layout.minReadableTile) {
    mode = "expand";
    tileSize = layout.minReadableTile;
    const readableScale = tileSize / layout.baseTileSize;
    gap = clamp(Math.round(layout.baseGap * readableScale), layout.minGap, layout.maxGap);
    driverOffset = clamp(
      Math.round(layout.baseDriverOffset * readableScale),
      layout.minDriverOffset,
      layout.maxDriverOffset,
    );
  }

  const gridW = cols * tileSize + (cols - 1) * gap;
  const gridH = rows * tileSize + (rows - 1) * gap;
  const neededW = layout.marginLeft + gridW + layout.marginRight;
  const neededH = layout.marginTop + gridH + layout.marginBottom;
  const width = mode === "expand" ? Math.max(targetWidth, neededW) : targetWidth;
  const height = mode === "expand" ? Math.max(targetHeight, neededH) : targetHeight;

  const freeW = width - layout.marginLeft - layout.marginRight - gridW;
  const freeH = height - layout.marginTop - layout.marginBottom - gridH;
  layout.width = width;
  layout.height = height;
  layout.tileSize = tileSize;
  layout.gap = gap;
  layout.driverOffset = driverOffset;
  layout.originX = layout.marginLeft + Math.max(0, Math.floor(freeW / 2));
  layout.originY = layout.marginTop + Math.max(0, Math.floor(freeH / 2));
  state.layoutMode = mode;

  svg.attr("viewBox", `0 0 ${layout.width} ${layout.height}`);
}

function tileRect(x, y) {
  const step = layout.tileSize + layout.gap;
  const px = layout.originX + x * step;
  const py = layout.originY + (state.maxY - y) * step;
  return { x: px, y: py, w: layout.tileSize, h: layout.tileSize };
}

function parseEndpoint(name) {
  if (!name || name === "None") {
    return null;
  }
  const tileMatch = /^Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(North|South|East|West)$/.exec(name);
  if (tileMatch) {
    return {
      kind: "tilePort",
      y: Number(tileMatch[1]),
      x: Number(tileMatch[2]),
      port: tileMatch[3],
      raw: name,
    };
  }
  const driverMatch = /^Driver\.Device(North|South|East|West)\[(\d+)\]$/.exec(name);
  if (driverMatch) {
    return {
      kind: "driver",
      side: driverMatch[1],
      idx: Number(driverMatch[2]),
      raw: name,
    };
  }
  return { kind: "unknown", raw: name };
}

function endpointPoint(ep) {
  if (!ep) {
    return null;
  }
  if (ep.kind === "tilePort") {
    const r = tileRect(ep.x, ep.y);
    if (ep.port === "North") return { x: r.x + r.w / 2, y: r.y, tile: tileKey(ep.x, ep.y) };
    if (ep.port === "South") return { x: r.x + r.w / 2, y: r.y + r.h, tile: tileKey(ep.x, ep.y) };
    if (ep.port === "West") return { x: r.x, y: r.y + r.h / 2, tile: tileKey(ep.x, ep.y) };
    if (ep.port === "East") return { x: r.x + r.w, y: r.y + r.h / 2, tile: tileKey(ep.x, ep.y) };
  }
  if (ep.kind === "driver") {
    const side = ep.side;
    const idx = ep.idx;
    if (side === "North" && idx <= state.maxX) {
      const r = tileRect(idx, state.maxY);
      return { x: r.x + r.w / 2, y: r.y - layout.driverOffset };
    }
    if (side === "South" && idx <= state.maxX) {
      const r = tileRect(idx, 0);
      return { x: r.x + r.w / 2, y: r.y + r.h + layout.driverOffset };
    }
    if (side === "West" && idx <= state.maxY) {
      const r = tileRect(0, idx);
      return { x: r.x - layout.driverOffset, y: r.y + r.h / 2 };
    }
    if (side === "East" && idx <= state.maxY) {
      const r = tileRect(state.maxX, idx);
      return { x: r.x + r.w + layout.driverOffset, y: r.y + r.h / 2 };
    }
  }
  return null;
}

function inferBounds(events) {
  let maxX = 0;
  let maxY = 0;
  for (const e of events) {
    if (Number.isInteger(e.X)) maxX = Math.max(maxX, e.X);
    if (Number.isInteger(e.Y)) maxY = Math.max(maxY, e.Y);
    for (const f of ["Src", "Dst", "From", "To"]) {
      if (!e[f]) continue;
      const ep = parseEndpoint(e[f]);
      if (ep && ep.kind === "tilePort") {
        maxX = Math.max(maxX, ep.x);
        maxY = Math.max(maxY, ep.y);
      }
    }
  }
  return { maxX, maxY };
}

function boundsFromProgramSpec(programSpec) {
  if (!programSpec) return null;
  const cols = Number(programSpec.arrayColumns);
  const rows = Number(programSpec.arrayRows);
  if (!Number.isFinite(cols) || !Number.isFinite(rows) || cols <= 0 || rows <= 0) return null;
  return {
    maxX: Math.max(0, Math.round(cols) - 1),
    maxY: Math.max(0, Math.round(rows) - 1),
  };
}

function boundsFromReportSpec(reportSpec) {
  if (!reportSpec?.grid) return null;
  const width = Number(reportSpec.grid.width);
  const height = Number(reportSpec.grid.height);
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return null;
  return {
    maxX: Math.max(0, Math.round(width) - 1),
    maxY: Math.max(0, Math.round(height) - 1),
  };
}

function resolveMeshBounds(events) {
  const yamlBounds = boundsFromProgramSpec(state.programSpec);
  if (yamlBounds) return yamlBounds;
  const traceBounds = inferBounds(events);
  const hasTraceBounds = traceBounds.maxX > 0 || traceBounds.maxY > 0;
  if (hasTraceBounds) return traceBounds;
  const reportBounds = boundsFromReportSpec(state.reportSpec);
  if (reportBounds) return reportBounds;
  return traceBounds;
}

function parseJsonLines(text) {
  const lines = text.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
  const rows = [];
  let lastTime = null;
  for (const line of lines) {
    try {
      const obj = JSON.parse(line);
      if (obj && Number.isFinite(Number(obj.Time))) {
        obj.Time = Math.round(Number(obj.Time));
        lastTime = obj.Time;
        rows.push(obj);
        continue;
      }
      // Some memory traces (e.g. LoadDirect/StoreDirect) may omit Time.
      // Reuse the latest observed cycle to keep them alignable in strict matching.
      if (obj && obj.msg === "Memory" && Number.isFinite(lastTime)) {
        obj.Time = lastTime;
        rows.push(obj);
      }
    } catch (_) {
      // Ignore malformed lines.
    }
  }
  return rows;
}

function indexByTime(events) {
  const byTime = new Map();
  let minTime = Number.POSITIVE_INFINITY;
  let maxTime = Number.NEGATIVE_INFINITY;
  for (const e of events) {
    const tKey = Math.round(Number(e.Time));
    if (!byTime.has(tKey)) byTime.set(tKey, []);
    byTime.get(tKey).push(e);
    minTime = Math.min(minTime, tKey);
    maxTime = Math.max(maxTime, tKey);
  }
  if (!Number.isFinite(minTime) || !Number.isFinite(maxTime)) {
    minTime = 0;
    maxTime = 0;
  }
  const sortedTimes = [...byTime.keys()].sort((a, b) => a - b);
  return { byTime, minTime, maxTime, sortedTimes };
}

function normalizeSlot(value, ii) {
  const v = Math.round(Number(value));
  if (!Number.isFinite(v)) return 0;
  if (ii > 0) {
    let slot = v % ii;
    if (slot < 0) slot += ii;
    return slot;
  }
  return v;
}

function signedDelta(actualSlot, expectedSlot, ii) {
  if (!Number.isFinite(actualSlot) || !Number.isFinite(expectedSlot)) return null;
  if (ii <= 0) return actualSlot - expectedSlot;
  const raw = normalizeSlot(actualSlot - expectedSlot, ii);
  if (raw === 0) return 0;
  return raw <= ii / 2 ? raw : raw - ii;
}

function sortCore(a, b) {
  if (b.y !== a.y) return b.y - a.y;
  return a.x - b.x;
}

function escapeHtml(text) {
  return String(text)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function formatDelta(delta) {
  if (delta == null || !Number.isFinite(Number(delta))) return "N/A";
  const v = Number(delta);
  return `${v >= 0 ? "+" : ""}${v}`;
}

function formatDataAsDecimal(value) {
  if (value == null) return null;
  const numeric = Number(value);
  if (Number.isFinite(numeric)) {
    if (Number.isInteger(numeric)) return String(numeric);
    return String(numeric);
  }
  const text = String(value).trim();
  return text.length > 0 ? text : null;
}

function numberOr(value, fallback = 0) {
  const v = Number(value);
  return Number.isFinite(v) ? v : fallback;
}

function integerOr(value, fallback = 0) {
  return Math.round(numberOr(value, fallback));
}

function nullableBool(value) {
  if (typeof value === "boolean") return value;
  return null;
}

function parseReportJson(text) {
  let raw;
  try {
    raw = JSON.parse(text);
  } catch (err) {
    throw new Error(`Invalid JSON: ${err.message}`);
  }
  if (!raw || typeof raw !== "object") {
    throw new Error("Report root must be an object.");
  }

  const tiles = (Array.isArray(raw.tiles) ? raw.tiles : [])
    .map((item) => {
      const x = integerOr(item?.x, NaN);
      const y = integerOr(item?.y, NaN);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
      const util = Math.max(0, Math.min(100, numberOr(item?.utilizationPct, 0)));
      return {
        x,
        y,
        coord: String(item?.coord || `(${x},${y})`),
        activeCycles: Math.max(0, integerOr(item?.activeCycles, 0)),
        utilizationPct: util,
        instCount: Math.max(0, integerOr(item?.instCount, 0)),
        sendCount: Math.max(0, integerOr(item?.sendCount, 0)),
        recvCount: Math.max(0, integerOr(item?.recvCount, 0)),
        memoryCount: Math.max(0, integerOr(item?.memoryCount, 0)),
        totalEvents: Math.max(0, integerOr(item?.totalEvents, 0)),
        backpressureCount: Math.max(0, integerOr(item?.backpressureCount, 0)),
      };
    })
    .filter(Boolean);

  const topHotTiles = (Array.isArray(raw.topHotTiles) ? raw.topHotTiles : [])
    .map((item) => {
      const x = integerOr(item?.x, NaN);
      const y = integerOr(item?.y, NaN);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
      return {
        x,
        y,
        coord: String(item?.coord || `(${x},${y})`),
        utilizationPct: Math.max(0, Math.min(100, numberOr(item?.utilizationPct, 0))),
        activeCycles: Math.max(0, integerOr(item?.activeCycles, 0)),
        totalEvents: Math.max(0, integerOr(item?.totalEvents, 0)),
      };
    })
    .filter(Boolean);
  const topBackpressureTiles = (Array.isArray(raw.topBackpressureTiles) ? raw.topBackpressureTiles : [])
    .map((item) => {
      const x = integerOr(item?.x, NaN);
      const y = integerOr(item?.y, NaN);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
      return {
        x,
        y,
        coord: String(item?.coord || `(${x},${y})`),
        backpressureCount: Math.max(0, integerOr(item?.backpressureCount, 0)),
      };
    })
    .filter(Boolean);

  const gridWidth = Math.max(0, integerOr(raw?.grid?.width, 0));
  const gridHeight = Math.max(0, integerOr(raw?.grid?.height, 0));
  const activeTileCount = Math.max(0, integerOr(raw?.activeTileCount, tiles.length));
  const fallbackHot = [...tiles]
    .sort((a, b) => {
      if (b.utilizationPct !== a.utilizationPct) return b.utilizationPct - a.utilizationPct;
      if (b.activeCycles !== a.activeCycles) return b.activeCycles - a.activeCycles;
      return b.totalEvents - a.totalEvents;
    })
    .slice(0, 8)
    .map((t) => ({
      x: t.x,
      y: t.y,
      coord: t.coord,
      utilizationPct: t.utilizationPct,
      activeCycles: t.activeCycles,
      totalEvents: t.totalEvents,
    }));
  const fallbackBackpressure = [...tiles]
    .filter((t) => t.backpressureCount > 0)
    .sort((a, b) => {
      if (b.backpressureCount !== a.backpressureCount) return b.backpressureCount - a.backpressureCount;
      if (b.totalEvents !== a.totalEvents) return b.totalEvents - a.totalEvents;
      return b.activeCycles - a.activeCycles;
    })
    .slice(0, 8)
    .map((t) => ({
      x: t.x,
      y: t.y,
      coord: t.coord,
      backpressureCount: t.backpressureCount,
    }));

  return {
    testName: String(raw.testName || ""),
    logPath: String(raw.logPath || ""),
    grid: {
      width: gridWidth,
      height: gridHeight,
    },
    totalCycles: Math.max(0, integerOr(raw.totalCycles, 0)),
    activeCyclesGlobal: Math.max(0, integerOr(raw.activeCyclesGlobal, 0)),
    idleCyclesGlobal: Math.max(0, integerOr(raw.idleCyclesGlobal, 0)),
    passed: nullableBool(raw.passed),
    mismatchCount: raw.mismatchCount == null ? null : Math.max(0, integerOr(raw.mismatchCount, 0)),
    instCount: Math.max(0, integerOr(raw.instCount, 0)),
    sendCount: Math.max(0, integerOr(raw.sendCount, 0)),
    recvCount: Math.max(0, integerOr(raw.recvCount, 0)),
    memoryCount: Math.max(0, integerOr(raw.memoryCount, 0)),
    totalEvents: Math.max(0, integerOr(raw.totalEvents, 0)),
    backpressureCount: Math.max(0, integerOr(raw.backpressureCount, 0)),
    backpressureCycles: Math.max(0, integerOr(raw.backpressureCycles, 0)),
    activeTileCount,
    tiles,
    topHotTiles: topHotTiles.length > 0 ? topHotTiles : fallbackHot,
    topBackpressureTiles: topBackpressureTiles.length > 0 ? topBackpressureTiles : fallbackBackpressure,
  };
}

function formatPercent(v) {
  if (!Number.isFinite(Number(v))) return "N/A";
  return `${Number(v).toFixed(1)}%`;
}

function renderReportView() {
  if (!controls.reportSummary || !controls.reportHotTiles || !controls.reportWarning) return;

  if (state.reportError) {
    controls.reportWarning.textContent = state.reportError;
    controls.reportWarning.className = "report-warning error";
    controls.reportSummary.innerHTML = "<div class=\"report-empty\">Report parse failed. Please provide a valid report JSON.</div>";
    controls.reportHotTiles.innerHTML = "";
    return;
  }

  if (!state.reportReady || !state.reportSpec) {
    controls.reportWarning.textContent = "Load a report JSON to see aggregate utilization and hot-tile stats.";
    controls.reportWarning.className = "report-warning";
    controls.reportSummary.innerHTML = "<div class=\"report-empty\">No report loaded.</div>";
    controls.reportHotTiles.innerHTML = "";
    return;
  }

  const report = state.reportSpec;
  const cards = [
    ["test", report.testName || "N/A"],
    ["passed", report.passed == null ? "N/A" : (report.passed ? "yes" : "no")],
    ["mismatch", report.mismatchCount == null ? "N/A" : report.mismatchCount],
    ["cycles", report.totalCycles],
    ["active(global)", report.activeCyclesGlobal],
    ["idle(global)", report.idleCyclesGlobal],
    ["active-tiles", report.activeTileCount],
    ["events", report.totalEvents],
    ["bp-count", report.backpressureCount],
    ["bp-cycles", report.backpressureCycles],
  ];
  controls.reportSummary.innerHTML = cards.map(
    ([k, v]) => `<div class="report-card"><div class="report-card-k">${escapeHtml(k)}</div><div class="report-card-v">${escapeHtml(v)}</div></div>`,
  ).join("");

  const meshW = state.maxX + 1;
  const meshH = state.maxY + 1;
  const reportW = integerOr(report.grid?.width, 0);
  const reportH = integerOr(report.grid?.height, 0);
  if (reportW > 0 && reportH > 0 && (reportW !== meshW || reportH !== meshH)) {
    controls.reportWarning.textContent =
      `grid mismatch: report=${reportW}x${reportH}, mesh=${meshW}x${meshH}. Heat overlay is clipped to current mesh.`;
    controls.reportWarning.className = "report-warning warn";
  } else {
    controls.reportWarning.textContent = `report loaded: ${reportW || "?"}x${reportH || "?"}, log=${report.logPath || "N/A"}`;
    controls.reportWarning.className = "report-warning";
  }

  const hotTiles = (Array.isArray(report.topHotTiles) ? report.topHotTiles : []).slice(0, 12);
  const bpTiles = (Array.isArray(report.topBackpressureTiles) ? report.topBackpressureTiles : []).slice(0, 12);
  const sections = [];
  if (hotTiles.length > 0) {
    const rows = hotTiles.map((tile, idx) =>
      `<tr><td>${idx + 1}</td><td>${escapeHtml(tile.coord || `(${tile.x},${tile.y})`)}</td><td>${escapeHtml(formatPercent(tile.utilizationPct))}</td><td>${escapeHtml(tile.activeCycles)}</td><td>${escapeHtml(tile.totalEvents)}</td></tr>`).join("");
    sections.push([
      "<div class=\"report-hot-title\">Top Hot Tiles</div>",
      "<table class=\"report-hot-table\">",
      "<thead><tr><th>#</th><th>coord</th><th>utilization</th><th>activeCycles</th><th>events</th></tr></thead>",
      `<tbody>${rows}</tbody>`,
      "</table>",
    ].join(""));
  } else {
    sections.push("<div class=\"report-empty\">No hot-tile entries.</div>");
  }
  if (bpTiles.length > 0) {
    const bpRows = bpTiles.map((tile, idx) =>
      `<tr><td>${idx + 1}</td><td>${escapeHtml(tile.coord || `(${tile.x},${tile.y})`)}</td><td>${escapeHtml(tile.backpressureCount)}</td></tr>`).join("");
    sections.push([
      "<div class=\"report-hot-title\">Top Backpressure Tiles</div>",
      "<table class=\"report-hot-table\">",
      "<thead><tr><th>#</th><th>coord</th><th>bp-count</th></tr></thead>",
      `<tbody>${bpRows}</tbody>`,
      "</table>",
    ].join(""));
  }
  controls.reportHotTiles.innerHTML = sections.join("");
}

function applyReportHeatOverlay() {
  if (!staticLayer) return;
  const heatTiles = staticLayer.selectAll(".tile-report-heat");
  if (!heatTiles || heatTiles.empty()) return;

  heatTiles
    .style("display", "none")
    .attr("opacity", 0);

  if (!state.reportReady || !state.reportSpec || state.reportHeatMetric !== "utilizationPct") return;

  const byCore = new Map();
  for (const tile of state.reportSpec.tiles || []) {
    byCore.set(tileKey(tile.x, tile.y), Math.max(0, Math.min(100, numberOr(tile.utilizationPct, 0))));
  }
  heatTiles.each(function (d) {
    const k = tileKey(d.x, d.y);
    if (!byCore.has(k)) return;
    const util = byCore.get(k);
    const alpha = 0.08 + (util / 100) * 0.52;
    d3.select(this)
      .style("display", null)
      .attr("opacity", alpha)
      .attr("data-util", util.toFixed(1));
  });
}

function loadReport(text) {
  try {
    state.reportSpec = parseReportJson(text);
    state.reportReady = true;
    state.reportError = "";
    if (!state.programSpec && state.events.length === 0) {
      const rb = boundsFromReportSpec(state.reportSpec);
      if (rb) {
        state.maxX = rb.maxX;
        state.maxY = rb.maxY;
        applyAdaptiveLayout();
        drawStaticScene();
      }
    }
    applyReportHeatOverlay();
    renderReportView();
  } catch (err) {
    state.reportSpec = null;
    state.reportReady = false;
    state.reportError = `Report JSON parse error: ${err.message}`;
    applyReportHeatOverlay();
    renderReportView();
  }
}

function abbrevOpLabel(slot, maxLen) {
  const len = maxLen ?? 5;
  const occTag = slot.occurrenceTotal > 1 ? `@${slot.sampleIndex}` : "";
  if (slot.opcode && String(slot.opcode).trim()) {
    const s = String(slot.opcode).trim();
    const head = s.length <= len ? s : s.slice(0, len);
    return `${head}${occTag}`;
  }
  return `#${slot.opId}${occTag}`;
}

function weightedMedian(samples) {
  if (!samples || samples.length === 0) return null;
  const sorted = [...samples]
    .filter((s) => Number.isFinite(s.value) && Number.isFinite(s.weight) && s.weight > 0)
    .sort((a, b) => a.value - b.value);
  if (sorted.length === 0) return null;
  const total = sorted.reduce((acc, s) => acc + s.weight, 0);
  let accWeight = 0;
  for (const s of sorted) {
    accWeight += s.weight;
    if (accWeight >= total / 2) return s.value;
  }
  return sorted[sorted.length - 1].value;
}

function boundaryLabel(x, y, bounds) {
  const tags = [];
  if (y === bounds.maxY) tags.push("N");
  if (y === bounds.minY) tags.push("S");
  if (x === bounds.minX) tags.push("W");
  if (x === bounds.maxX) tags.push("E");
  return tags.length > 0 ? tags.join("") : "Inner";
}

function computeDeltaRebased(rawDelta, corePhaseOffset, ii) {
  if (!Number.isFinite(rawDelta) || !Number.isFinite(corePhaseOffset)) return null;
  return signedDelta(rawDelta - corePhaseOffset, 0, ii);
}

function summarizeTimingCell(items) {
  const statusCounts = { onTime: 0, early: 0, late: 0, missing: 0 };
  let hasFirstDivergence = false;
  let propagatedCount = 0;
  let maxAbsDelta = 0;
  for (const item of items) {
    if (item.status === "on-time") statusCounts.onTime += 1;
    if (item.status === "early") statusCounts.early += 1;
    if (item.status === "late") statusCounts.late += 1;
    if (item.status === "missing") statusCounts.missing += 1;
    if (item.firstDivergence) hasFirstDivergence = true;
    if (item.propagated) propagatedCount += 1;
    if (Number.isFinite(item.delta)) {
      maxAbsDelta = Math.max(maxAbsDelta, Math.abs(Number(item.delta)));
    }
  }
  const anomalyCount = statusCounts.early + statusCounts.late + statusCounts.missing;
  const anomalyScore =
    statusCounts.missing * 4 +
    statusCounts.late * 3 +
    statusCounts.early * 2 +
    (hasFirstDivergence ? 2 : 0) +
    (propagatedCount > 0 ? 1 : 0);
  let dominantStatus = "on-time";
  if (statusCounts.missing > 0) dominantStatus = "missing";
  else if (statusCounts.late > 0) dominantStatus = "late";
  else if (statusCounts.early > 0) dominantStatus = "early";
  return {
    statusCounts,
    dominantStatus,
    anomalyCount,
    anomalyScore,
    hasAnomaly: anomalyCount > 0,
    hasFirstDivergence,
    opCount: items.length,
    maxAbsDelta,
  };
}

function buildTimingHeatmap(view) {
  const cells = new Map();
  let maxScore = 1;
  for (const c of view.columns) {
    for (const slot of view.slots) {
      const cellKey = `${c.coreKey}|${slot}`;
      const items = view.cellMap.get(cellKey) || [];
      const summary = summarizeTimingCell(items);
      cells.set(cellKey, {
        cellKey,
        coreKey: c.coreKey,
        x: c.x,
        y: c.y,
        slot,
        ...summary,
      });
      maxScore = Math.max(maxScore, summary.anomalyScore);
    }
  }
  return { cells, maxScore };
}

function buildPhaseExplain(view) {
  const xs = view.columns.map((c) => c.x);
  const ys = view.columns.map((c) => c.y);
  const bounds = {
    minX: xs.length > 0 ? Math.min(...xs) : 0,
    maxX: xs.length > 0 ? Math.max(...xs) : 0,
    minY: ys.length > 0 ? Math.min(...ys) : 0,
    maxY: ys.length > 0 ? Math.max(...ys) : 0,
  };
  const coreMap = new Map();
  const boundarySamples = [];
  const innerSamples = [];

  for (const c of view.columns) {
    const label = boundaryLabel(c.x, c.y, bounds);
    const isBoundary = label !== "Inner";
    const phaseOffset = Number.isFinite(c.phaseOffset) ? c.phaseOffset : null;
    const confidence = Number.isFinite(c.phaseConfidence) ? c.phaseConfidence : 0;
    const detail = {
      isBoundary,
      boundaryLabel: label,
      phaseOffset,
      phaseConfidence: confidence,
      modeCount: c.modeCount,
    };
    coreMap.set(c.coreKey, detail);
    if (phaseOffset == null) continue;
    const sample = { value: phaseOffset, weight: Math.max(1, c.modeCount || 0) };
    if (isBoundary) {
      boundarySamples.push(sample);
    } else {
      innerSamples.push(sample);
    }
  }

  const boundaryPhase = weightedMedian(boundarySamples);
  const innerPhase = weightedMedian(innerSamples);
  const phaseGap = Number.isFinite(boundaryPhase) && Number.isFinite(innerPhase)
    ? signedDelta(boundaryPhase, innerPhase, view.ii)
    : null;

  return {
    coreMap,
    boundaryPhase,
    innerPhase,
    phaseGap,
  };
}

function inferIngressSidesFromTrace(events) {
  const sides = new Set();
  for (const e of events) {
    if (e.msg !== "DataFlow" || e.Behavior !== "FeedIn" || !e.To) continue;
    const ep = parseEndpoint(e.To);
    if (ep && ep.kind === "tilePort") {
      sides.add(ep.port);
    }
  }
  if (sides.size === 0) {
    sides.add("North");
    sides.add("West");
  }
  return [...sides];
}

function distanceToIngress(x, y, bounds, ingressSides) {
  const d = [];
  for (const side of ingressSides) {
    if (side === "North") d.push(bounds.maxY - y);
    if (side === "South") d.push(y - bounds.minY);
    if (side === "West") d.push(x - bounds.minX);
    if (side === "East") d.push(bounds.maxX - x);
  }
  if (d.length === 0) return 0;
  // In GEMM-like wavefronts, readiness is dominated by the slower upstream stream.
  return Math.max(...d);
}

function statusFromDelta(deltaValue, missing) {
  if (missing) return "missing";
  if (!Number.isFinite(deltaValue)) return "missing";
  if (deltaValue === 0) return "on-time";
  return deltaValue < 0 ? "early" : "late";
}

function getModelOffset(modelItem, compModel) {
  if (!modelItem) return null;
  if (compModel === "distance") return modelItem.distanceOffset;
  if (compModel === "fitted") return modelItem.fittedOffset;
  return modelItem.hybridOffset;
}

function getCompDeltaByModel(slot, compModel) {
  if (compModel === "distance") return slot.deltaCompDistance;
  if (compModel === "fitted") return slot.deltaCompFitted;
  return slot.deltaCompHybrid;
}

function getCompStatusByModel(slot, compModel) {
  if (compModel === "distance") return slot.statusCompDistance;
  if (compModel === "fitted") return slot.statusCompFitted;
  return slot.statusCompHybrid;
}

function summarizeModelBoundary(ii, phaseExplain, coreOffsets, modelKey) {
  const boundarySamples = [];
  const innerSamples = [];
  for (const [coreKey, offsetInfo] of coreOffsets.entries()) {
    const offset = offsetInfo[modelKey];
    if (!Number.isFinite(offset)) continue;
    const meta = phaseExplain.coreMap.get(coreKey);
    const weight = Math.max(1, Number(meta?.modeCount || 0));
    const sample = { value: Number(offset), weight };
    if (meta?.isBoundary) boundarySamples.push(sample);
    else innerSamples.push(sample);
  }
  const boundary = weightedMedian(boundarySamples);
  const inner = weightedMedian(innerSamples);
  const gap = Number.isFinite(boundary) && Number.isFinite(inner)
    ? signedDelta(boundary, inner, ii)
    : null;
  return { boundary, inner, gap };
}

function buildCompensationModels(view, phaseExplain, events) {
  const ingressSides = inferIngressSidesFromTrace(events);
  const xs = view.columns.map((c) => c.x);
  const ys = view.columns.map((c) => c.y);
  const bounds = {
    minX: xs.length > 0 ? Math.min(...xs) : 0,
    maxX: xs.length > 0 ? Math.max(...xs) : 0,
    minY: ys.length > 0 ? Math.min(...ys) : 0,
    maxY: ys.length > 0 ? Math.max(...ys) : 0,
  };

  const rawDistancePhaseSamples = [];
  for (const c of view.columns) {
    const dist = distanceToIngress(c.x, c.y, bounds, ingressSides);
    const phase = view.ii > 0 ? normalizeSlot(dist, view.ii) : dist;
    rawDistancePhaseSamples.push({ value: phase, weight: 1, coreKey: c.coreKey, rawDist: dist });
  }
  const center = weightedMedian(rawDistancePhaseSamples);
  const coreOffsets = new Map();
  for (const c of view.columns) {
    const phaseMeta = phaseExplain.coreMap.get(c.coreKey) || {};
    const row = rawDistancePhaseSamples.find((v) => v.coreKey === c.coreKey);
    const rawPhase = row ? row.value : 0;
    const distanceOffset = view.ii > 0
      ? signedDelta(rawPhase, Number.isFinite(center) ? center : 0, view.ii)
      : rawPhase - (Number.isFinite(center) ? center : 0);
    const fittedOffset = Number.isFinite(phaseMeta.phaseOffset) ? Number(phaseMeta.phaseOffset) : null;
    const fittedConfidence = Number.isFinite(phaseMeta.phaseConfidence) ? Number(phaseMeta.phaseConfidence) : 0;
    const hybridOffset = (Number.isFinite(fittedOffset) && fittedConfidence >= 0.4)
      ? fittedOffset
      : distanceOffset;
    coreOffsets.set(c.coreKey, {
      distanceOffset,
      fittedOffset,
      hybridOffset,
      fittedConfidence,
      ingressDistance: row ? row.rawDist : 0,
    });
  }

  return {
    ingressSides,
    coreOffsets,
    models: {
      distance: summarizeModelBoundary(view.ii, phaseExplain, coreOffsets, "distanceOffset"),
      fitted: summarizeModelBoundary(view.ii, phaseExplain, coreOffsets, "fittedOffset"),
      hybrid: summarizeModelBoundary(view.ii, phaseExplain, coreOffsets, "hybridOffset"),
    },
  };
}

function alignSlotAtOrAfter(startTime, expectedSlot, ii) {
  const t0 = Math.round(Number(startTime || 0));
  if (ii <= 0) return expectedSlot;
  const slot = normalizeSlot(expectedSlot, ii);
  let offset = slot - normalizeSlot(t0, ii);
  if (offset < 0) offset += ii;
  return t0 + offset;
}

function buildTimelineLanes(view, visibleColumns, phaseExplain, compensation) {
  const lanes = [];
  let minT = Number.POSITIVE_INFINITY;
  let maxT = Number.NEGATIVE_INFINITY;
  let totalSlots = 0;

  for (const c of visibleColumns) {
    const coreMeta = phaseExplain.coreMap.get(c.coreKey) || {};
    const expectedSlots = [];
    const actualSlots = [];
    const compMeta = compensation.coreOffsets.get(c.coreKey) || null;
    for (const item of c.items) {
      const samples = (Array.isArray(item.allSamples) && item.allSamples.length > 0)
        ? item.allSamples
        : [null];
      const occurrenceTotal = samples.length;
      for (let sampleIdx = 0; sampleIdx < samples.length; sampleIdx += 1) {
        const sample = samples[sampleIdx];
        const hasActual = sample && Number.isFinite(sample.time);
        const actualTime = hasActual ? Math.round(sample.time) : null;
        const deltaStrict = hasActual ? sample.delta : item.delta;
        const statusStrict = hasActual ? sample.status : item.status;
        const missing = !hasActual;

        let expectedTime = null;
        if (hasActual && Number.isFinite(deltaStrict)) {
          // Expand expected per actual occurrence to cover full trace length.
          expectedTime = Math.round(actualTime - deltaStrict);
        } else if (Number.isFinite(item.firstTime) && Number.isFinite(item.delta)) {
          expectedTime = Math.round(item.firstTime - item.delta);
        } else if (view.ii > 0) {
          expectedTime = alignSlotAtOrAfter(state.minTime, item.expectedSlot, view.ii);
        } else {
          expectedTime = item.expectedSlot;
        }

        const deltaRebased = computeDeltaRebased(deltaStrict, coreMeta.phaseOffset, view.ii);
        const deltaCompDistance = computeDeltaRebased(deltaStrict, compMeta?.distanceOffset, view.ii);
        const deltaCompFitted = computeDeltaRebased(deltaStrict, compMeta?.fittedOffset, view.ii);
        const deltaCompHybrid = computeDeltaRebased(deltaStrict, compMeta?.hybridOffset, view.ii);
        const statusCompDistance = statusFromDelta(deltaCompDistance, missing);
        const statusCompFitted = statusFromDelta(deltaCompFitted, missing);
        const statusCompHybrid = statusFromDelta(deltaCompHybrid, missing);
        const cellKey = `${c.coreKey}|${item.expectedSlot}`;
        const slot = {
          coreKey: c.coreKey,
          x: c.x,
          y: c.y,
          expectedSlot: item.expectedSlot,
          opId: item.id,
          opcode: item.opcode || "",
          status: statusStrict,
          statusStrict,
          expectedTime,
          actualTime,
          delta: deltaStrict,
          deltaStrict,
          deltaRebased,
          deltaCompDistance,
          deltaCompFitted,
          deltaCompHybrid,
          statusCompDistance,
          statusCompFitted,
          statusCompHybrid,
          compDistanceOffset: compMeta?.distanceOffset ?? null,
          compFittedOffset: compMeta?.fittedOffset ?? null,
          compHybridOffset: compMeta?.hybridOffset ?? null,
          firstDivergence: item.firstDivergence,
          propagated: item.propagated,
          cellKey,
          sampleIdx,
          sampleIndex: sampleIdx + 1,
          occurrenceTotal,
          samplePred: hasActual ? sample.pred : null,
          sampleSource: hasActual ? String(sample.source || "Unknown") : null,
        };
        expectedSlots.push(slot);
        if (Number.isFinite(actualTime)) {
          actualSlots.push(slot);
        }

        if (Number.isFinite(expectedTime)) {
          minT = Math.min(minT, expectedTime);
          maxT = Math.max(maxT, expectedTime);
          totalSlots += 1;
        }
        if (Number.isFinite(actualTime)) {
          minT = Math.min(minT, actualTime);
          maxT = Math.max(maxT, actualTime);
          totalSlots += 1;
        }
      }
    }
    lanes.push({
      coreKey: c.coreKey,
      x: c.x,
      y: c.y,
      modeDelta: c.modeDelta,
      modeCount: c.modeCount,
      statusCounts: c.statusCounts,
      phaseOffset: coreMeta.phaseOffset ?? null,
      phaseConfidence: coreMeta.phaseConfidence ?? 0,
      compDistanceOffset: compMeta?.distanceOffset ?? null,
      compFittedOffset: compMeta?.fittedOffset ?? null,
      compHybridOffset: compMeta?.hybridOffset ?? null,
      compFittedConfidence: compMeta?.fittedConfidence ?? 0,
      boundaryLabel: coreMeta.boundaryLabel || "Inner",
      isBoundary: Boolean(coreMeta.isBoundary),
      expectedSlots,
      actualSlots,
    });
  }

  if (!Number.isFinite(minT) || !Number.isFinite(maxT)) {
    minT = state.minTime;
    maxT = state.maxTime;
  }
  minT = Math.min(minT, state.minTime);
  maxT = Math.max(maxT, state.maxTime);
  if (minT === maxT) maxT = minT + 1;

  return {
    lanes,
    timeMin: minT,
    timeMax: maxT,
    totalSlots,
  };
}

function tickStepByRange(span) {
  if (span <= 40) return 2;
  if (span <= 120) return 5;
  if (span <= 360) return 10;
  if (span <= 900) return 25;
  return 50;
}

function renderTimelineSvg(_view, timeline) {
  const wrap = controls.timingGrid;
  if (!wrap) return;
  wrap.innerHTML = "";

  const baselineView = ["strict", "compensated", "split"].includes(state.timingBaselineView)
    ? state.timingBaselineView
    : "strict";
  const compModel = ["distance", "fitted", "hybrid"].includes(state.timingCompModel)
    ? state.timingCompModel
    : "hybrid";

  const fullMin = timeline.timeMin;
  const fullMax = timeline.timeMax;
  const fullSpan = Math.max(1, fullMax - fullMin + 1);
  const minWindow = 1;
  const windowSize = clamp(
    Math.round(Number(state.timingWindowSize || Math.min(120, fullSpan))),
    minWindow,
    fullSpan,
  );
  const startMax = Math.max(fullMin, fullMax - windowSize + 1);
  const windowStart = clamp(
    Math.round(Number(state.timingWindowStart || fullMin)),
    fullMin,
    startMax,
  );
  const windowEnd = windowStart + windowSize - 1;
  state.timingWindowStart = windowStart;
  state.timingWindowSize = windowSize;

  if (controls.timingWindowStart) {
    controls.timingWindowStart.min = String(fullMin);
    controls.timingWindowStart.max = String(startMax);
    controls.timingWindowStart.value = String(windowStart);
    controls.timingWindowStart.disabled = fullSpan <= 1;
  }
  if (controls.timingWindowSize) {
    controls.timingWindowSize.min = String(minWindow);
    controls.timingWindowSize.max = String(fullSpan);
    controls.timingWindowSize.value = String(windowSize);
  }
  if (controls.timingWindowStartLabel) {
    controls.timingWindowStartLabel.textContent = `T${windowStart}-T${windowEnd}`;
  }
  if (controls.timingWindowSizeLabel) {
    controls.timingWindowSizeLabel.textContent = `${windowSize} cycles`;
  }

  const zoomY = clamp(Number(state.timingZoomY || 1), 0.6, 4);
  state.timingZoomX = 1;
  state.timingZoomY = zoomY;
  if (controls.timingZoomY) {
    controls.timingZoomY.value = String(Math.round(zoomY * 100));
  }
  if (controls.timingZoomYLabel) {
    controls.timingZoomYLabel.textContent = `${zoomY.toFixed(2)}x`;
  }

  const leftPad = 242;
  const rightPad = 30;
  const topPad = 30;
  const bottomPad = 38;
  const slotHeight = clamp(Math.round(8 * zoomY), 6, 34);
  const subLaneGap = clamp(Math.round(5 * zoomY), 3, 22);
  const laneGap = clamp(Math.round(10 * zoomY), 6, 46);
  const splitView = baselineView === "split";
  const baseLaneRows = splitView ? 3 : 2;
  const availableCoreKeys = new Set(timeline.lanes.map((lane) => lane.coreKey));
  const selectedIoKeys = new Set([...(state.timingIoWaveExpandedCoreKeys || [])]);
  const ioWaveExpandedKeys = state.timingIoWaveExpandAll
    ? new Set([...availableCoreKeys])
    : new Set([...selectedIoKeys].filter((key) => availableCoreKeys.has(key)));
  const rowStep = slotHeight + subLaneGap;
  const laneData = [];
  let yCursor = topPad;
  for (let idx = 0; idx < timeline.lanes.length; idx += 1) {
    const lane = timeline.lanes[idx];
    const hasIoRows = ioWaveExpandedKeys.has(lane.coreKey);
    const laneRows = baseLaneRows + (hasIoRows ? 2 : 0);
    const laneHeight = laneRows * slotHeight + (laneRows - 1) * subLaneGap;
    laneData.push({
      ...lane,
      idx,
      hasIoRows,
      laneRows,
      yBase: yCursor,
      yExpected: yCursor,
      yStrict: yCursor + rowStep,
      yComp: yCursor + rowStep * 2,
      yIoIn: hasIoRows ? yCursor + rowStep * baseLaneRows : null,
      yIoOut: hasIoRows ? yCursor + rowStep * (baseLaneRows + 1) : null,
    });
    yCursor += laneHeight + laneGap;
  }
  const laneCount = Math.max(1, laneData.length);
  const plotH = Math.max(1, yCursor - topPad - laneGap);
  const wrapWidth = Math.max(860, Math.round(wrap.clientWidth || 0) - 2);
  const plotW = Math.max(620, wrapWidth - leftPad - rightPad);
  const width = leftPad + plotW + rightPad;
  const height = topPad + plotH + bottomPad;
  const labelFontSize = clamp(Math.round(slotHeight * 0.72), 7, 13);
  const labelMinWidth = Math.max(14, labelFontSize * 2 + 2);

  const svgEl = d3.create("svg")
    .attr("id", "timingTimelineSvg")
    .attr("class", "timing-timeline-svg")
    .attr("viewBox", `0 0 ${width} ${height}`)
    .attr("width", width)
    .attr("height", height);

  const xScale = d3.scaleLinear()
    .domain([windowStart, windowEnd + 1])
    .range([leftPad, leftPad + plotW]);
  state.timingViewport = {
    fullMin,
    fullMax,
    fullSpan,
    windowStart,
    windowSize,
    windowEnd,
    leftPad,
    plotW,
  };

  const ticks = [];
  for (let t = windowStart; t <= windowEnd; t += 1) ticks.push(t);

  // Cycle boundaries (X): dashed vertical lines only; no other grid
  const grid = svgEl.append("g").attr("class", "timeline-grid");
  for (const t of ticks) {
    const x = xScale(t);
    grid.append("line")
      .attr("x1", x).attr("x2", x)
      .attr("y1", topPad).attr("y2", topPad + plotH)
      .attr("class", "timeline-cycle-sep");
    grid.append("text")
      .attr("x", x + 2)
      .attr("y", topPad + plotH + 16)
      .attr("class", "timeline-tick")
      .text(`T${t}`);
  }
  svgEl.append("line")
    .attr("x1", leftPad).attr("x2", leftPad + plotW)
    .attr("y1", topPad + plotH).attr("y2", topPad + plotH)
    .attr("class", "timeline-axis");

  // Core boundaries (Y): one dashed/dark line between each core for easier row matching
  const coreSep = svgEl.append("g").attr("class", "timeline-core-seps");
  for (let idx = 1; idx < laneCount; idx += 1) {
    const y = laneData[idx].yBase;
    coreSep.append("line")
      .attr("x1", leftPad)
      .attr("x2", leftPad + plotW)
      .attr("y1", y)
      .attr("y2", y)
      .attr("class", "timeline-core-sep");
  }

  // Lane labels (no inner sub-row grid lines)
  const lanesG = svgEl.append("g").attr("class", "timeline-lanes");
  for (const lane of laneData) {
    const modelOffset = compModel === "distance"
      ? lane.compDistanceOffset
      : (compModel === "fitted" ? lane.compFittedOffset : lane.compHybridOffset);
    const phaseText = state.showPhaseExplain
      ? ` SΔ=${lane.phaseOffset == null ? "N/A" : formatDelta(lane.phaseOffset)} CΔ=${formatDelta(modelOffset)}`
      : "";
    lanesG.append("text")
      .attr("x", 8)
      .attr("y", lane.yExpected + slotHeight + 1)
      .attr("class", [
        "timeline-core-label",
        lane.isBoundary ? "boundary" : "",
        lane.hasIoRows ? "io-expanded" : "",
        state.timingFocusedCoreKey === lane.coreKey ? "focused" : "",
      ].filter(Boolean).join(" "))
      .attr("data-core-key", lane.coreKey)
      .attr("title", `Click: focus core (${lane.x},${lane.y}) | Double-click: toggle IO wave`)
      .text(`(${lane.x},${lane.y}) ${lane.boundaryLabel}${phaseText}`);
    lanesG.append("text")
      .attr("x", leftPad - 64)
      .attr("y", lane.yExpected + slotHeight - 1)
      .attr("class", "timeline-lane-tag")
      .text("E");
    lanesG.append("text")
      .attr("x", leftPad - 64)
      .attr("y", lane.yStrict + slotHeight - 1)
      .attr("class", "timeline-lane-tag")
      .text(splitView ? "S" : (baselineView === "strict" ? "A" : "C"));
    if (splitView) {
      lanesG.append("text")
        .attr("x", leftPad - 64)
        .attr("y", lane.yComp + slotHeight - 1)
        .attr("class", "timeline-lane-tag")
        .text("C");
    }
    if (lane.hasIoRows) {
      lanesG.append("text")
        .attr("x", leftPad - 64)
        .attr("y", lane.yIoIn + slotHeight - 1)
        .attr("class", "timeline-lane-tag timeline-io-tag-in")
        .text("IN");
      lanesG.append("text")
        .attr("x", leftPad - 64)
        .attr("y", lane.yIoOut + slotHeight - 1)
        .attr("class", "timeline-lane-tag timeline-io-tag-out")
        .text("OUT");
    }
  }

  const slotG = svgEl.append("g").attr("class", "timeline-rects");
  const keepSlot = (slot) => {
    if (!state.timingAnomalyOnly) return true;
    const strictAnomaly = slot.statusStrict !== "on-time";
    const compAnomaly = getCompStatusByModel(slot, compModel) !== "on-time";
    if (baselineView === "strict") return strictAnomaly;
    if (baselineView === "compensated") return compAnomaly;
    return strictAnomaly || compAnomaly;
  };
  const applyStackLayout = (items, keyOf, tieBreakOf) => {
    const buckets = new Map();
    for (const item of items) {
      const key = keyOf(item);
      const arr = buckets.get(key) || [];
      arr.push(item);
      buckets.set(key, arr);
    }
    for (const group of buckets.values()) {
      group.sort((a, b) => tieBreakOf(a) - tieBreakOf(b));
      const total = group.length;
      if (total <= 1) {
        group[0].stackIndex = 0;
        group[0].stackTotal = 1;
        continue;
      }
      for (let i = 0; i < group.length; i += 1) {
        group[i].stackIndex = i;
        group[i].stackTotal = total;
      }
    }
  };
  const resolveStackGeometry = (baseY, baseH, stackIndex, stackTotal) => {
    if (!Number.isFinite(stackTotal) || stackTotal <= 1) {
      return { y: baseY, h: baseH };
    }
    // Keep all stacked blocks visible within one cycle slot row.
    const gap = 1;
    const innerH = Math.max(2, Math.floor((baseH - gap * (stackTotal - 1)) / stackTotal));
    const y = baseY + stackIndex * (innerH + gap);
    return { y, h: innerH };
  };
  const summarizeWaveValues = (values, maxItems = 2) => {
    const arr = Array.isArray(values) ? values : [];
    if (arr.length === 0) return "";
    const shown = arr.slice(0, maxItems).map((v) => shortText(v, 7));
    const remain = arr.length - shown.length;
    if (remain > 0) shown.push(`+${remain}`);
    return shown.join(",");
  };
  const ioBusPath = (xLeft, xRight, yTop, yBottom) => {
    const yMid = (yTop + yBottom) / 2;
    const w = Math.max(1, xRight - xLeft);
    const edge = Math.min(5, Math.max(1, Math.round(w * 0.22)));
    return [
      `M${xLeft + edge},${yTop}`,
      `L${xRight - edge},${yTop}`,
      `L${xRight},${yMid}`,
      `L${xRight - edge},${yBottom}`,
      `L${xLeft + edge},${yBottom}`,
      `L${xLeft},${yMid}`,
      "Z",
    ].join(" ");
  };
  const drawIoWaveRow = (lane, yPos, direction) => {
    if (!Number.isFinite(yPos)) return;
    const byTime = state.coreIoWaveByTime.get(lane.coreKey);
    if (!byTime) return;
    for (let t = windowStart; t <= windowEnd; t += 1) {
      const entry = byTime.get(t);
      const values = direction === "in" ? entry?.inVals : entry?.outVals;
      if (!Array.isArray(values) || values.length === 0) continue;
      const xLeft = xScale(t);
      const xRight = xScale(t + 1);
      const w = Math.max(1, xRight - xLeft);
      slotG.append("path")
        .attr("d", ioBusPath(xLeft, xRight, yPos, yPos + slotHeight))
        .attr("class", `timeline-io-bus ${direction === "in" ? "timeline-io-bus-in" : "timeline-io-bus-out"}`)
        .attr(
          "title",
          `${direction === "in" ? "Input" : "Output"} core=(${lane.x},${lane.y}) t=${t} values=${values.join(",")}`,
        );
      if (w >= labelMinWidth + 6) {
        slotG.append("text")
          .attr("x", xLeft + w / 2)
          .attr("y", yPos + slotHeight / 2)
          .attr("text-anchor", "middle")
          .attr("dominant-baseline", "middle")
          .attr("class", `timeline-io-bus-label ${direction === "in" ? "timeline-io-bus-label-in" : "timeline-io-bus-label-out"}`)
          .attr("font-size", labelFontSize)
          .text(summarizeWaveValues(values));
      }
    }
  };
  const drawActualRow = (lane, yPos, rowMode) => {
    const drawables = [];
    for (const slot of lane.expectedSlots) {
      if (!keepSlot(slot)) continue;
      const rowStatus = rowMode === "strict" ? slot.statusStrict : getCompStatusByModel(slot, compModel);
      let drawTime = null;
      let cls = "actual-ok";
      if (rowStatus === "missing") {
        drawTime = slot.expectedTime;
        cls = "missing";
      } else if (Number.isFinite(slot.actualTime)) {
        drawTime = slot.actualTime;
        if (rowMode === "strict") {
          cls = rowStatus === "on-time" ? "actual-ok" : "actual-bad";
        } else {
          cls = rowStatus === "on-time" ? "actual-comp-ok" : "actual-comp-bad";
        }
      }
      if (!Number.isFinite(drawTime)) continue;
      if (drawTime < windowStart || drawTime > windowEnd) continue;
      drawables.push({ slot, rowStatus, drawTime, cls });
    }
    applyStackLayout(
      drawables,
      (d) => `${lane.coreKey}|${rowMode}|${d.drawTime}`,
      (d) => (d.slot.opId * 10000) + (d.slot.sampleIndex || 0),
    );
    for (const d of drawables) {
      const { slot, rowStatus, drawTime, cls, stackIndex = 0, stackTotal = 1 } = d;
      const x0 = xScale(drawTime);
      const x1 = xScale(drawTime + 1);
      const w = Math.max(1, Math.floor(x1 - x0 - 1));
      const selected = state.timingSelectedCell === slot.cellKey ? "selected" : "";
      const geom = resolveStackGeometry(yPos, slotHeight, stackIndex, stackTotal);
      slotG.append("rect")
        .attr("x", x0 + 0.5)
        .attr("y", geom.y)
        .attr("width", w)
        .attr("height", geom.h)
        .attr("class", `timeline-rect ${cls} ${selected}`)
        .attr("data-timing-cell", slot.cellKey)
        .attr(
          "title",
          `${rowMode === "strict" ? "Strict" : `Comp(${compModel})`} #${slot.opId}[${slot.sampleIndex}/${slot.occurrenceTotal}] status=${rowStatus} t=${Number.isFinite(slot.actualTime) ? slot.actualTime : "N/A"} deltaS=${formatDelta(slot.deltaStrict)} deltaC=${formatDelta(getCompDeltaByModel(slot, compModel))}`,
        );
      if (w >= labelMinWidth && geom.h >= 8) {
        slotG.append("text")
          .attr("x", x0 + 0.5 + w / 2)
          .attr("y", geom.y + geom.h / 2)
          .attr("text-anchor", "middle")
          .attr("dominant-baseline", "middle")
          .attr("class", "timeline-rect-label timeline-rect-label-actual")
          .attr("font-size", labelFontSize)
          .text(abbrevOpLabel(slot));
      }

      if (state.timingSelectedCell === slot.cellKey && Number.isFinite(slot.actualTime) && Number.isFinite(slot.expectedTime)) {
        const xe = xScale(slot.expectedTime) + Math.max(1, Math.floor((xScale(slot.expectedTime + 1) - xScale(slot.expectedTime)) / 2));
        const xa = xScale(slot.actualTime) + Math.max(1, Math.floor((xScale(slot.actualTime + 1) - xScale(slot.actualTime)) / 2));
        const linkClass = rowMode === "strict"
          ? (rowStatus === "on-time" ? "ok" : "bad")
          : (rowStatus === "on-time" ? "comp-ok" : "comp-bad");
        slotG.append("line")
          .attr("x1", xe).attr("y1", lane.yExpected + slotHeight)
          .attr("x2", xa).attr("y2", geom.y)
          .attr("class", `timeline-link ${linkClass}`);
      }
    }
  };

  for (const lane of laneData) {
    const expectedDrawables = [];
    for (const slot of lane.expectedSlots) {
      if (!keepSlot(slot)) continue;
      if (!Number.isFinite(slot.expectedTime)) continue;
      if (slot.expectedTime < windowStart || slot.expectedTime > windowEnd) continue;
      expectedDrawables.push({ slot, drawTime: slot.expectedTime });
    }
    applyStackLayout(
      expectedDrawables,
      (d) => `${lane.coreKey}|expected|${d.drawTime}`,
      (d) => (d.slot.opId * 10000) + (d.slot.sampleIndex || 0),
    );
    for (const d of expectedDrawables) {
      const { slot, stackIndex = 0, stackTotal = 1 } = d;
      const x0 = xScale(slot.expectedTime);
      const x1 = xScale(slot.expectedTime + 1);
      const w = Math.max(1, Math.floor(x1 - x0 - 1));
      const selected = state.timingSelectedCell === slot.cellKey ? "selected" : "";
      const geom = resolveStackGeometry(lane.yExpected, slotHeight, stackIndex, stackTotal);
      slotG.append("rect")
        .attr("x", x0 + 0.5)
        .attr("y", geom.y)
        .attr("width", w)
        .attr("height", geom.h)
        .attr("class", `timeline-rect expected ${selected}`)
        .attr("data-timing-cell", slot.cellKey)
        .attr(
          "title",
          `Expected #${slot.opId}[${slot.sampleIndex}/${slot.occurrenceTotal}] (${slot.opcode || "N/A"}) t=${slot.expectedTime} deltaS=${formatDelta(slot.deltaStrict)} deltaC=${formatDelta(getCompDeltaByModel(slot, compModel))}`,
        );
      if (w >= labelMinWidth && geom.h >= 8) {
        slotG.append("text")
          .attr("x", x0 + 0.5 + w / 2)
          .attr("y", geom.y + geom.h / 2)
          .attr("text-anchor", "middle")
          .attr("dominant-baseline", "middle")
          .attr("class", "timeline-rect-label timeline-rect-label-expected")
          .attr("font-size", labelFontSize)
          .text(abbrevOpLabel(slot));
      }
    }
    if (baselineView === "strict") {
      drawActualRow(lane, lane.yStrict, "strict");
    } else if (baselineView === "compensated") {
      drawActualRow(lane, lane.yStrict, "comp");
    } else {
      drawActualRow(lane, lane.yStrict, "strict");
      drawActualRow(lane, lane.yComp, "comp");
    }
    if (lane.hasIoRows) {
      drawIoWaveRow(lane, lane.yIoIn, "in");
      drawIoWaveRow(lane, lane.yIoOut, "out");
    }
  }

  // Legend
  const legend = svgEl.append("g").attr("class", "timeline-legend").attr("transform", `translate(${leftPad},14)`);
  const legendItems = baselineView === "strict"
    ? [
      ["Expected slot", "timeline-legend-exp"],
      ["Strict on-time", "timeline-legend-act-ok"],
      ["Strict mismatch", "timeline-legend-act-bad"],
      ["Missing", "timeline-legend-missing"],
    ]
    : (baselineView === "compensated"
      ? [
        ["Expected slot", "timeline-legend-exp"],
        [compModel === "hybrid" ? "Hybrid on-time" : `Comp(${compModel}) on-time`, "timeline-legend-comp-ok"],
        [compModel === "hybrid" ? "Hybrid mismatch" : `Comp(${compModel}) mismatch`, "timeline-legend-comp-bad"],
        ["Missing", "timeline-legend-missing"],
      ]
      : [
        ["Expected slot", "timeline-legend-exp"],
        ["Strict on-time", "timeline-legend-act-ok"],
        ["Strict mismatch", "timeline-legend-act-bad"],
        [`Comp(${compModel}) on-time`, "timeline-legend-comp-ok"],
        [`Comp(${compModel}) mismatch`, "timeline-legend-comp-bad"],
        ["Missing", "timeline-legend-missing"],
      ]);
  if (laneData.some((lane) => lane.hasIoRows)) {
    legendItems.push(["IN bus", "timeline-legend-io-in"]);
    legendItems.push(["OUT bus", "timeline-legend-io-out"]);
  }
  const legendGap = 132;
  legendItems.forEach((it, i) => {
    const gx = i * legendGap;
    legend.append("rect").attr("x", gx).attr("y", -4).attr("width", 10).attr("height", 8).attr("class", it[1]);
    legend.append("text").attr("x", gx + 14).attr("y", 4).attr("class", "timeline-legend-text").text(it[0]);
  });

  wrap.appendChild(svgEl.node());
}

function timelineZoomAnchorTimeFromWheel(event) {
  const vp = state.timingViewport;
  if (!vp) {
    return Number(state.timingWindowStart || 0) + Number(state.timingWindowSize || 1) / 2;
  }
  const svgElement = document.getElementById("timingTimelineSvg");
  if (!svgElement) {
    return vp.windowStart + vp.windowSize / 2;
  }
  const rect = svgElement.getBoundingClientRect();
  const localX = event.clientX - rect.left;
  const ratio = clamp((localX - vp.leftPad) / Math.max(1, vp.plotW), 0, 0.999);
  return vp.windowStart + ratio * vp.windowSize;
}

function handleTimelineCtrlWheelZoom(event) {
  if (!event.ctrlKey) return;
  if (state.events.length === 0 || !state.programSpec) return;
  event.preventDefault();

  const vp = state.timingViewport || {
    fullMin: state.minTime,
    fullMax: state.maxTime,
    fullSpan: Math.max(1, state.maxTime - state.minTime + 1),
  };
  const fullMin = vp.fullMin;
  const fullMax = vp.fullMax;
  const fullSpan = Math.max(1, vp.fullSpan || (fullMax - fullMin + 1));
  const minWindow = 1;
  const oldWindow = Math.max(1, Number(state.timingWindowSize || Math.min(120, fullSpan)));
  const oldStart = Number(state.timingWindowStart || fullMin);
  const zoomIn = event.deltaY < 0;
  const anchorTime = timelineZoomAnchorTimeFromWheel(event);
  const nextWindow = clamp(
    Math.round(oldWindow * (zoomIn ? 0.88 : 1.14)),
    minWindow,
    fullSpan,
  );
  const anchorRatio = clamp((anchorTime - oldStart) / oldWindow, 0, 1);
  const startMax = Math.max(fullMin, fullMax - nextWindow + 1);
  const nextStart = clamp(
    Math.round(anchorTime - anchorRatio * nextWindow),
    fullMin,
    startMax,
  );
  const factor = zoomIn ? 1.08 : 1 / 1.08;
  state.timingWindowStart = nextStart;
  state.timingWindowSize = nextWindow;
  state.timingZoomX = 1;
  state.timingZoomY = clamp(Number(state.timingZoomY || 1) * factor, 0.6, 4);
  renderTimingView();
}

function getTimelineSvgSize(svgElement) {
  const vb = svgElement.getAttribute("viewBox");
  if (vb) {
    const parts = vb.trim().split(/\s+/).map(Number);
    if (parts.length === 4 && Number.isFinite(parts[2]) && Number.isFinite(parts[3])) {
      return { width: parts[2], height: parts[3] };
    }
  }
  const width = Number(svgElement.getAttribute("width")) || svgElement.clientWidth || 1200;
  const height = Number(svgElement.getAttribute("height")) || svgElement.clientHeight || 800;
  return { width, height };
}

function parseMaxSide() {
  const fallback = 4096;
  if (!controls.timingExportMaxSide) return fallback;
  const v = Math.round(Number(controls.timingExportMaxSide.value));
  if (!Number.isFinite(v)) return fallback;
  return clamp(v, 512, 16000);
}

function timelineExportCss() {
  return `
.timing-timeline-svg { background: #fffaf0; }
.timeline-axis { stroke: #8f846d; stroke-width: 1; }
.timeline-cycle-sep { stroke: #c4b89a; stroke-width: 1; stroke-dasharray: 3 2; }
.timeline-core-sep { stroke: #7a6f58; stroke-width: 1.2; stroke-dasharray: 4 3; }
.timeline-tick { fill: #7a6f58; font-size: 10px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.timeline-core-label { fill: #5a5347; font-size: 11px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.timeline-core-label.boundary { font-weight: 700; }
.timeline-core-label.focused { fill: #1f4eb5; font-weight: 700; text-decoration: underline; }
.timeline-core-label.io-expanded { fill: #6a2b96; text-decoration: underline; text-decoration-style: dashed; }
.timeline-lane-tag { fill: #7f7460; font-size: 10px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.timeline-io-tag-in { fill: #2d6cdf; font-weight: 700; }
.timeline-io-tag-out { fill: #8f2ac7; font-weight: 700; }
.timeline-rect.expected { fill: #f4f4f4; stroke: #8f8f8f; stroke-width: 0.8; }
.timeline-rect.actual-ok { fill: #2a7f62; stroke: #1f604a; stroke-width: 0.7; }
.timeline-rect.actual-bad { fill: #d62828; stroke: #8f1717; stroke-width: 0.7; }
.timeline-rect.actual-comp-ok { fill: #2d6cdf; stroke: #1d4a97; stroke-width: 0.8; opacity: 0.84; }
.timeline-rect.actual-comp-bad { fill: #9b2ce0; stroke: #5d178a; stroke-width: 0.85; opacity: 0.9; }
.timeline-rect.missing { fill: #f4f4f4; stroke: #7a7a7a; stroke-width: 1.1; stroke-dasharray: 2 1; }
.timeline-rect.selected { stroke-width: 1.8; }
.timeline-io-bus { stroke-width: 0.9; }
.timeline-io-bus-in { fill: #deebff; stroke: #7da3ea; }
.timeline-io-bus-out { fill: #f1e1ff; stroke: #b589dd; }
.timeline-io-bus-label { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; pointer-events: none; font-size: 7px; }
.timeline-io-bus-label-in { fill: #214a9c; }
.timeline-io-bus-label-out { fill: #6d2094; }
.timeline-missing { stroke: #7a7a7a; stroke-width: 1.2; }
.timeline-link.ok { stroke: rgba(54, 132, 103, 0.45); stroke-width: 0.9; }
.timeline-link.bad { stroke: rgba(214, 40, 40, 0.72); stroke-width: 1.2; }
.timeline-link.comp-ok { stroke: rgba(45, 108, 223, 0.5); stroke-width: 0.9; stroke-dasharray: 2 1; }
.timeline-link.comp-bad { stroke: rgba(155, 44, 224, 0.78); stroke-width: 1.2; stroke-dasharray: 2 1; }
.timeline-legend-text { fill: #615a4f; font-size: 10px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.timeline-legend-exp { fill: #fff; stroke: #8c8c8c; }
.timeline-legend-act-ok { fill: #2a7f62; }
.timeline-legend-act-bad { fill: #d62828; }
.timeline-legend-missing { fill: #f4f4f4; stroke: #7a7a7a; stroke-dasharray: 2 1; }
.timeline-legend-comp-ok { fill: #2d6cdf; }
.timeline-legend-comp-bad { fill: #9b2ce0; }
.timeline-legend-io-in { fill: #deebff; stroke: #7da3ea; }
.timeline-legend-io-out { fill: #f1e1ff; stroke: #b589dd; }
.timeline-rect-label { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 7px; }
.timeline-rect-label-expected { fill: #444; }
.timeline-rect-label-actual { fill: #fff; }`;
}

function exportTimelinePng() {
  const svgElement = document.getElementById("timingTimelineSvg");
  if (!svgElement) return;
  const size = getTimelineSvgSize(svgElement);
  const maxSide = parseMaxSide();
  const scale = Math.min(1, maxSide / Math.max(size.width, size.height));
  const outW = Math.max(1, Math.round(size.width * scale));
  const outH = Math.max(1, Math.round(size.height * scale));

  const serializer = new XMLSerializer();
  const clone = svgElement.cloneNode(true);
  const styleEl = document.createElementNS("http://www.w3.org/2000/svg", "style");
  styleEl.textContent = timelineExportCss();
  clone.insertBefore(styleEl, clone.firstChild);
  let source = serializer.serializeToString(clone);
  if (!source.includes("xmlns=\"http://www.w3.org/2000/svg\"")) {
    source = source.replace("<svg", "<svg xmlns=\"http://www.w3.org/2000/svg\"");
  }
  const svgBlob = new Blob([source], { type: "image/svg+xml;charset=utf-8" });
  const url = URL.createObjectURL(svgBlob);
  const img = new Image();
  img.onload = () => {
    const canvas = document.createElement("canvas");
    canvas.width = outW;
    canvas.height = outH;
    const ctx = canvas.getContext("2d");
    if (!ctx) {
      URL.revokeObjectURL(url);
      return;
    }
    ctx.fillStyle = "#fffdf7";
    ctx.fillRect(0, 0, outW, outH);
    ctx.drawImage(img, 0, 0, outW, outH);
    canvas.toBlob((blob) => {
      if (!blob) {
        URL.revokeObjectURL(url);
        return;
      }
      const dlUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = dlUrl;
      a.download = "timeline.png";
      a.click();
      URL.revokeObjectURL(dlUrl);
      URL.revokeObjectURL(url);
    }, "image/png");
  };
  img.onerror = () => {
    URL.revokeObjectURL(url);
  };
  img.src = url;
}

function splitCellKey(cellKey) {
  const pivot = cellKey.lastIndexOf("|");
  if (pivot <= 0) return { coreKey: "", slot: 0 };
  return {
    coreKey: cellKey.slice(0, pivot),
    slot: Number(cellKey.slice(pivot + 1)),
  };
}

function buildCoreIoWaveByTime(events) {
  const byCore = new Map();
  const ensure = (coreKey, time) => {
    if (!byCore.has(coreKey)) byCore.set(coreKey, new Map());
    const byTime = byCore.get(coreKey);
    if (!byTime.has(time)) byTime.set(time, { inVals: [], outVals: [] });
    return byTime.get(time);
  };
  for (const e of events) {
    if (e.msg !== "DataFlow") continue;
    const time = Math.round(Number(e.Time));
    if (!Number.isFinite(time)) continue;
    const value = formatDataAsDecimal(e.Data);
    if (e.Behavior === "FeedIn" || e.Behavior === "Recv") {
      const dst = parseEndpoint(e.Behavior === "FeedIn" ? e.To : e.Dst);
      if (dst?.kind !== "tilePort") continue;
      const cell = ensure(tileKey(dst.x, dst.y), time);
      if (value != null) cell.inVals.push(value);
      continue;
    }
    if (e.Behavior === "Send" || e.Behavior === "Collect") {
      const src = parseEndpoint(e.Behavior === "Collect" ? e.From : e.Src);
      if (src?.kind !== "tilePort") continue;
      const cell = ensure(tileKey(src.x, src.y), time);
      if (value != null) cell.outVals.push(value);
    }
  }
  return byCore;
}

function refreshCoreFocusControl(columns) {
  if (!controls.timingCoreFocus) return;
  const options = [
    { value: "", label: "All cores" },
    ...columns.map((c) => ({ value: c.coreKey, label: `(${c.x},${c.y})` })),
  ];
  controls.timingCoreFocus.innerHTML = options
    .map((opt) => `<option value="${escapeHtml(opt.value)}">${escapeHtml(opt.label)}</option>`)
    .join("");
  const hasFocused = columns.some((c) => c.coreKey === state.timingFocusedCoreKey);
  if (!hasFocused) state.timingFocusedCoreKey = null;
  controls.timingCoreFocus.value = state.timingFocusedCoreKey || "";
}

function refreshIoWaveCoreControl(columns) {
  if (!controls.timingIoWaveCore || !controls.timingIoWaveAll) return;
  const validKeys = new Set(columns.map((c) => c.coreKey));
  const expanded = new Set(
    [...(state.timingIoWaveExpandedCoreKeys || [])].filter((key) => validKeys.has(key)),
  );
  state.timingIoWaveExpandedCoreKeys = expanded;

  const options = columns.map((c) => ({ value: c.coreKey, label: `(${c.x},${c.y})` }));
  controls.timingIoWaveCore.innerHTML = options
    .map((opt) => `<option value="${escapeHtml(opt.value)}">${escapeHtml(opt.label)}</option>`)
    .join("");
  const shouldSelectAll = Boolean(state.timingIoWaveExpandAll && columns.length > 0);
  if (state.timingIoWaveExpandAll && columns.length === 0) {
    state.timingIoWaveExpandAll = false;
  }
  const selectedKeys = shouldSelectAll
    ? new Set(columns.map((c) => c.coreKey))
    : expanded;
  for (const opt of controls.timingIoWaveCore.options) {
    opt.selected = selectedKeys.has(opt.value);
  }
  controls.timingIoWaveAll.checked = shouldSelectAll;
}

function renderTimingDrilldown(view, heatmap, phaseExplain, compensation) {
  if (!controls.timingDrilldown) return;
  if (!state.timingSelectedCell || !heatmap.cells.has(state.timingSelectedCell)) {
    controls.timingDrilldown.innerHTML =
      "<div class=\"timing-drill-empty\">Click a timeline mark to inspect operation-level details.</div>";
    return;
  }
  const selected = heatmap.cells.get(state.timingSelectedCell);
  const { slot } = splitCellKey(state.timingSelectedCell);
  const items = view.cellMap.get(state.timingSelectedCell) || [];
  const corePhase = phaseExplain.coreMap.get(selected.coreKey);
  const coreComp = compensation.coreOffsets.get(selected.coreKey) || null;
  const compOffset = getModelOffset(coreComp, state.timingCompModel);
  const corePhaseText = corePhase?.phaseOffset == null ? "N/A" : formatDelta(corePhase.phaseOffset);
  const coreCompText = compOffset == null ? "N/A" : formatDelta(compOffset);
  const confPct = `${Math.round((corePhase?.phaseConfidence || 0) * 100)}%`;
  const summary = [
    `core=(${selected.x},${selected.y})`,
    `edge=${corePhase?.boundaryLabel || "N/A"}`,
    `slot=s${Number.isFinite(slot) ? slot : "N/A"}`,
    `ops=${selected.opCount}`,
    `anomaly=${selected.anomalyCount}`,
    `strictPhase=${corePhaseText}`,
    `comp(${state.timingCompModel})=${coreCompText}`,
    `conf=${confPct}`,
  ].join(" | ");

  if (items.length === 0) {
    controls.timingDrilldown.innerHTML =
      `<div class="timing-drill-head">${escapeHtml(summary)}</div><div class="timing-drill-empty">No expected operations in this cell.</div>`;
    return;
  }

  let html = `<div class="timing-drill-head">${escapeHtml(summary)}</div>`;
  html += "<div class=\"timing-drill-list\">";
  for (const item of items) {
    const rowCls = [
      "timing-drill-row",
      item.status,
      item.firstDivergence ? "first-divergence" : "",
      item.propagated ? "propagated" : "",
    ].filter(Boolean).join(" ");
    const opLabel = `#${item.id} ${item.opcode || "N/A"}`;
    const deltaComp = computeDeltaRebased(item.delta, compOffset, view.ii);
    const statusComp = statusFromDelta(deltaComp, item.status === "missing");
    const compLabel = state.timingCompModel === "hybrid" ? "hybrid" : `comp(${state.timingCompModel})`;
    const allSamples = Array.isArray(item.allSamples) ? item.allSamples : [];
    const sourceCounts = new Map();
    for (const s of allSamples) {
      const src = String(s?.source || "Unknown");
      sourceCounts.set(src, (sourceCounts.get(src) || 0) + 1);
    }
    const sourceSummary = sourceCounts.size > 0
      ? [...sourceCounts.entries()].map(([k, v]) => `${k}*${v}`).join(",")
      : "N/A";
    const sampleRange = allSamples.length > 0
      ? `${allSamples[0].time}..${allSamples[allSamples.length - 1].time}`
      : "N/A";
    const samplePreview = allSamples.length > 0
      ? allSamples.slice(0, 4).map((s, idx) => `${idx + 1}:${s.time}:${s.source || "Unknown"}`).join(",")
      : "N/A";
    const fields = [
      `statusComp=${statusComp}`,
      `deltaComp(${compLabel})=${formatDelta(deltaComp)}`,
      `exp=s${item.expectedSlot}`,
      `act=${item.actualSlot == null ? "N/A" : `s${item.actualSlot}`}`,
      `statusStrict=${item.status} (reference)`,
      `deltaStrict=${formatDelta(item.delta)} (reference)`,
      `deltaPhaseRebased=${formatDelta(computeDeltaRebased(item.delta, corePhase?.phaseOffset, view.ii))}`,
      `time=${item.firstTime == null ? "N/A" : item.firstTime}`,
      `samples=${item.sampleCount}`,
      `sourceSummary=${sourceSummary}`,
      `sampleRange=${sampleRange}`,
      `samplePreview=${samplePreview}`,
      `div=${item.firstDivergence ? "yes" : "no"}`,
    ].join(" | ");
    html += `<div class="${rowCls}" title="${escapeHtml(fields)}"><span class="drill-op">${escapeHtml(opLabel)}</span><span class="drill-meta">${escapeHtml(fields)}</span></div>`;
  }
  html += "</div>";
  controls.timingDrilldown.innerHTML = html;
}

function renderFocusedCoreMini(view, timeline) {
  if (!controls.timingCoreMini) return;
  const focusedKey = state.timingFocusedCoreKey;
  if (!focusedKey) {
    controls.timingCoreMini.innerHTML =
      "<div class=\"timing-core-mini-empty\">Click a Y-axis core label or use the core selector to focus one core.</div>";
    return;
  }
  const core = view.columns.find((c) => c.coreKey === focusedKey);
  const lane = timeline.lanes.find((l) => l.coreKey === focusedKey);
  if (!core || !lane) {
    controls.timingCoreMini.innerHTML =
      "<div class=\"timing-core-mini-empty\">Focused core is not visible under current filters.</div>";
    return;
  }
  const sourceCounts = new Map();
  for (const item of core.items) {
    const samples = Array.isArray(item.allSamples) ? item.allSamples : [];
    for (const s of samples) {
      const src = String(s?.source || "Unknown");
      sourceCounts.set(src, (sourceCounts.get(src) || 0) + 1);
    }
  }
  const sourceText = sourceCounts.size > 0
    ? [...sourceCounts.entries()].map(([k, v]) => `${k}*${v}`).join(" | ")
    : "N/A";
  const windowStart = Number(state.timingWindowStart || timeline.timeMin);
  const windowEnd = windowStart + Number(state.timingWindowSize || 1) - 1;
  const compModel = ["distance", "fitted", "hybrid"].includes(state.timingCompModel)
    ? state.timingCompModel
    : "hybrid";
  const rows = lane.expectedSlots
    .filter((slot) => {
      const inExp = Number.isFinite(slot.expectedTime) && slot.expectedTime >= windowStart && slot.expectedTime <= windowEnd;
      const inAct = Number.isFinite(slot.actualTime) && slot.actualTime >= windowStart && slot.actualTime <= windowEnd;
      return inExp || inAct;
    })
    .sort((a, b) => {
      const ta = Number.isFinite(a.actualTime) ? a.actualTime : a.expectedTime;
      const tb = Number.isFinite(b.actualTime) ? b.actualTime : b.expectedTime;
      if (ta !== tb) return ta - tb;
      if (a.opId !== b.opId) return a.opId - b.opId;
      return (a.sampleIndex || 0) - (b.sampleIndex || 0);
    })
    .slice(0, 28);
  const listHtml = rows.length > 0
    ? rows.map((slot) => {
      const occ = `[${slot.sampleIndex}/${slot.occurrenceTotal}]`;
      const src = slot.sampleSource || "N/A";
      const strict = slot.statusStrict;
      const comp = getCompStatusByModel(slot, compModel);
      const line = `#${slot.opId}${occ} ${slot.opcode || "N/A"} expT=${slot.expectedTime ?? "N/A"} actT=${slot.actualTime ?? "N/A"} strict=${strict} comp=${comp} src=${src}`;
      return `<div class="timing-core-mini-row">${escapeHtml(line)}</div>`;
    }).join("")
    : "<div class=\"timing-core-mini-empty\">No blocks from this core in current window.</div>";

  controls.timingCoreMini.innerHTML = [
    `<div class="timing-core-mini-head">focused-core=(${core.x},${core.y}) | window=T${windowStart}..T${windowEnd} | sources=${escapeHtml(sourceText)}</div>`,
    `<div class="timing-core-mini-list">${listHtml}</div>`,
  ].join("");
}

function parseProgramYaml(text) {
  if (!window.jsyaml) {
    throw new Error("js-yaml is unavailable in current page.");
  }
  const parsed = window.jsyaml.load(text);
  const cfg = parsed?.array_config;
  if (!cfg || !Array.isArray(cfg.cores)) {
    throw new Error("Program YAML must contain array_config.cores.");
  }

  const ii = Math.max(0, Math.round(Number(cfg.compiled_ii || 0)));
  const arrayColumns = Math.round(Number(cfg.columns));
  const arrayRows = Math.round(Number(cfg.rows));
  const hasArraySize = Number.isFinite(arrayColumns) && Number.isFinite(arrayRows) && arrayColumns > 0 && arrayRows > 0;
  const expectedOps = [];
  const coreSet = new Map();

  for (const core of cfg.cores) {
    const x = Number(core.column);
    const y = Number(core.row);
    if (!Number.isFinite(x) || !Number.isFinite(y)) continue;
    const coreKey = tileKey(x, y);
    if (!coreSet.has(coreKey)) coreSet.set(coreKey, { coreKey, x, y });

    const entries = Array.isArray(core.entries) ? core.entries : [];
    for (const entry of entries) {
      const groups = Array.isArray(entry.instructions) ? entry.instructions : [];
      for (const ig of groups) {
        const fallbackSlot = normalizeSlot(ig.index_per_ii || 0, ii);
        const ops = Array.isArray(ig.operations) ? ig.operations : [];
        for (const op of ops) {
          const id = Number(op.id);
          if (!Number.isFinite(id)) continue;
          const rawTimeStep = Number(op.time_step);
          const hasTimeStep = Number.isFinite(rawTimeStep);
          const expectedSlot = normalizeSlot(hasTimeStep ? rawTimeStep : fallbackSlot, ii);
          expectedOps.push({
            coreKey,
            x,
            y,
            id: Math.round(id),
            opcode: String(op.opcode || ""),
            expectedSlot,
            rawTimeStep: hasTimeStep ? Math.round(rawTimeStep) : null,
          });
        }
      }
    }
  }

  const columns = [...coreSet.values()].sort(sortCore);
  const maxSlot = expectedOps.reduce((acc, op) => Math.max(acc, op.expectedSlot), 0);
  const slots = ii > 0
    ? Array.from({ length: ii }, (_v, idx) => idx)
    : Array.from({ length: maxSlot + 1 }, (_v, idx) => idx);

  return {
    ii,
    expectedOps,
    columns,
    slots,
    arrayColumns: hasArraySize ? arrayColumns : null,
    arrayRows: hasArraySize ? arrayRows : null,
  };
}

function buildActualByCoreAndId(events, ii) {
  const actualByCore = new Map();
  for (const e of events) {
    const isInst = e.msg === "Inst";
    const isMemoryDirect = e.msg === "Memory"
      && (String(e.Behavior || "") === "LoadDirect" || String(e.Behavior || "") === "StoreDirect");
    if (!isInst && !isMemoryDirect) continue;
    if (!Number.isFinite(Number(e.Time))
      || !Number.isFinite(Number(e.ID))
      || !Number.isFinite(Number(e.X))
      || !Number.isFinite(Number(e.Y))) {
      continue;
    }
    const coreKey = tileKey(Number(e.X), Number(e.Y));
    if (!actualByCore.has(coreKey)) actualByCore.set(coreKey, new Map());
    const byId = actualByCore.get(coreKey);
    const id = Math.round(Number(e.ID));
    if (!byId.has(id)) byId.set(id, []);
    byId.get(id).push({
      time: Math.round(Number(e.Time)),
      slot: normalizeSlot(e.Time, ii),
      pred: e.Pred,
      source: isInst ? "Inst" : String(e.Behavior || "MemoryDirect"),
    });
  }
  for (const byId of actualByCore.values()) {
    for (const samples of byId.values()) {
      samples.sort((a, b) => a.time - b.time);
    }
  }
  return actualByCore;
}

function buildStrictTimingView(programSpec, events) {
  const actualByCoreAndId = buildActualByCoreAndId(events, programSpec.ii);
  const cellMap = new Map();
  const byCore = new Map();

  for (const op of programSpec.expectedOps) {
    const actuals = actualByCoreAndId.get(op.coreKey)?.get(op.id) || [];
    const first = actuals.length > 0 ? actuals[0] : null;
    const delta = first ? signedDelta(first.slot, op.expectedSlot, programSpec.ii) : null;
    const status = !first ? "missing" : (delta === 0 ? "on-time" : (delta < 0 ? "early" : "late"));
    const allSamples = actuals.map((sample, sampleIdx) => {
      const sampleDelta = signedDelta(sample.slot, op.expectedSlot, programSpec.ii);
      const sampleStatus = sampleDelta === 0 ? "on-time" : (sampleDelta < 0 ? "early" : "late");
      return {
        ...sample,
        sampleIdx,
        delta: sampleDelta,
        status: sampleStatus,
      };
    });
    const compareItem = {
      ...op,
      actualSlot: first ? first.slot : null,
      firstTime: first ? first.time : null,
      sampleCount: actuals.length,
      delta,
      status,
      allSamples,
      firstDivergence: false,
      propagated: false,
    };

    if (!byCore.has(op.coreKey)) byCore.set(op.coreKey, []);
    byCore.get(op.coreKey).push(compareItem);

    const cellKey = `${op.coreKey}|${op.expectedSlot}`;
    if (!cellMap.has(cellKey)) cellMap.set(cellKey, []);
    cellMap.get(cellKey).push(compareItem);
  }

  const columns = programSpec.columns.map((c) => {
    const items = byCore.get(c.coreKey) || [];
    items.sort((a, b) => {
      const ta = Number.isFinite(a.firstTime) ? a.firstTime : Number.POSITIVE_INFINITY;
      const tb = Number.isFinite(b.firstTime) ? b.firstTime : Number.POSITIVE_INFINITY;
      if (ta !== tb) return ta - tb;
      if (a.expectedSlot !== b.expectedSlot) return a.expectedSlot - b.expectedSlot;
      return a.id - b.id;
    });

    let hasDivergence = false;
    let lastDelta = null;
    for (const item of items) {
      if (item.status === "on-time") continue;
      if (item.status === "missing") {
        if (!hasDivergence) {
          item.firstDivergence = true;
          hasDivergence = true;
        } else {
          item.propagated = true;
        }
        continue;
      }
      if (!hasDivergence || item.delta !== lastDelta) {
        item.firstDivergence = true;
        hasDivergence = true;
      } else {
        item.propagated = true;
      }
      lastDelta = item.delta;
    }

    const deltaCounts = new Map();
    const statusCounts = { onTime: 0, early: 0, late: 0, missing: 0 };
    for (const item of items) {
      if (item.status === "on-time") statusCounts.onTime += 1;
      if (item.status === "early") statusCounts.early += 1;
      if (item.status === "late") statusCounts.late += 1;
      if (item.status === "missing") statusCounts.missing += 1;
      if (item.status === "early" || item.status === "late") {
        const k = String(item.delta);
        deltaCounts.set(k, (deltaCounts.get(k) || 0) + 1);
      }
    }
    let modeDelta = 0;
    let modeCount = 0;
    for (const [k, v] of deltaCounts.entries()) {
      if (v > modeCount) {
        modeCount = v;
        modeDelta = Number(k);
      }
    }

    return {
      ...c,
      items,
      modeDelta,
      modeCount,
      statusCounts,
      earlyLateCount: statusCounts.early + statusCounts.late,
      phaseOffset: modeCount > 0 ? modeDelta : null,
      phaseConfidence: (statusCounts.early + statusCounts.late) > 0
        ? modeCount / (statusCounts.early + statusCounts.late)
        : 0,
    };
  });

  for (const items of cellMap.values()) {
    items.sort((a, b) => a.id - b.id);
  }

  return {
    ii: programSpec.ii,
    slots: programSpec.slots,
    columns,
    cellMap,
  };
}

function renderTimingView() {
  if (!controls.timingGrid || !controls.timingSummary) return;
  if (!state.programSpec) {
    controls.timingSummary.textContent = "Load program YAML to enable strict timing comparison.";
    controls.timingGrid.innerHTML = "";
    if (controls.timingCoreFocus) controls.timingCoreFocus.innerHTML = "<option value=\"\">All cores</option>";
    if (controls.timingIoWaveCore) controls.timingIoWaveCore.innerHTML = "";
    if (controls.timingIoWaveAll) controls.timingIoWaveAll.checked = false;
    if (controls.timingDrilldown) {
      controls.timingDrilldown.innerHTML =
        "<div class=\"timing-drill-empty\">Load YAML and trace, then click a timeline mark for details.</div>";
    }
    if (controls.timingCoreMini) {
      controls.timingCoreMini.innerHTML =
        "<div class=\"timing-core-mini-empty\">Focus one core to inspect local trace details.</div>";
    }
    return;
  }
  if (state.events.length === 0) {
    controls.timingSummary.textContent = "Load trace log to populate timing comparison.";
    controls.timingGrid.innerHTML = "";
    if (controls.timingCoreFocus) controls.timingCoreFocus.innerHTML = "<option value=\"\">All cores</option>";
    if (controls.timingIoWaveCore) controls.timingIoWaveCore.innerHTML = "";
    if (controls.timingIoWaveAll) controls.timingIoWaveAll.checked = false;
    if (controls.timingDrilldown) {
      controls.timingDrilldown.innerHTML =
        "<div class=\"timing-drill-empty\">Load YAML and trace, then click a timeline mark for details.</div>";
    }
    if (controls.timingCoreMini) {
      controls.timingCoreMini.innerHTML =
        "<div class=\"timing-core-mini-empty\">Focus one core to inspect local trace details.</div>";
    }
    return;
  }

  const view = buildStrictTimingView(state.programSpec, state.events);
  state.timingRows = view.columns;
  state.timingColumns = view.slots;
  state.timingReady = true;
  refreshCoreFocusControl(view.columns);
  refreshIoWaveCoreControl(view.columns);
  const heatmap = buildTimingHeatmap(view);
  const phaseExplain = buildPhaseExplain(view);
  const compensation = buildCompensationModels(view, phaseExplain, state.events);

  const totals = { onTime: 0, early: 0, late: 0, missing: 0 };
  for (const c of view.columns) {
    totals.onTime += c.statusCounts.onTime;
    totals.early += c.statusCounts.early;
    totals.late += c.statusCounts.late;
    totals.missing += c.statusCounts.missing;
  }
  const filterText = state.timingAnomalyOnly ? "filter=anomaly-only" : "filter=all";
  const boundaryText = state.timingBoundaryOnly ? "scope=boundary-only" : "scope=all-cores";
  const focusedCoreText = state.timingFocusedCoreKey ? `focus=${state.timingFocusedCoreKey}` : "focus=all-cores";
  const phaseText = state.showPhaseExplain
    ? `phase(boundary=${formatDelta(phaseExplain.boundaryPhase)} inner=${formatDelta(phaseExplain.innerPhase)} gap=${formatDelta(phaseExplain.phaseGap)})`
    : "phase(hidden)";
  const compModel = ["distance", "fitted", "hybrid"].includes(state.timingCompModel)
    ? state.timingCompModel
    : "hybrid";
  const modelSummary = compensation.models[compModel];
  const compTotals = { onTime: 0, early: 0, late: 0, missing: 0 };
  for (const c of view.columns) {
    const compMeta = compensation.coreOffsets.get(c.coreKey) || null;
    const compOffset = getModelOffset(compMeta, compModel);
    for (const item of c.items) {
      const deltaComp = computeDeltaRebased(item.delta, compOffset, view.ii);
      const statusComp = statusFromDelta(deltaComp, item.status === "missing");
      if (statusComp === "on-time") compTotals.onTime += 1;
      if (statusComp === "early") compTotals.early += 1;
      if (statusComp === "late") compTotals.late += 1;
      if (statusComp === "missing") compTotals.missing += 1;
    }
  }
  controls.timingSummary.textContent =
    `strict baseline | ii=${view.ii || "N/A"} | on-time=${totals.onTime} early=${totals.early} late=${totals.late} missing=${totals.missing} | comp(${compModel}) on-time=${compTotals.onTime} early=${compTotals.early} late=${compTotals.late} missing=${compTotals.missing} gap=${formatDelta(modelSummary?.gap)} | view=${state.timingBaselineView} | ${filterText} | ${boundaryText} | ${focusedCoreText} | ingress=${compensation.ingressSides.join("+")} | ${phaseText}`;

  let visibleColumns = view.columns;
  if (state.timingBoundaryOnly) {
    visibleColumns = visibleColumns.filter((c) => phaseExplain.coreMap.get(c.coreKey)?.isBoundary);
  }
  if (state.timingFocusedCoreKey) {
    visibleColumns = visibleColumns.filter((c) => c.coreKey === state.timingFocusedCoreKey);
  }
  const visibleCoreSet = new Set(visibleColumns.map((c) => c.coreKey));

  if (state.timingSelectedCell && !heatmap.cells.has(state.timingSelectedCell)) {
    state.timingSelectedCell = null;
  }
  if (state.timingSelectedCell) {
    const selectedCoreKey = splitCellKey(state.timingSelectedCell).coreKey;
    if (!visibleCoreSet.has(selectedCoreKey)) {
      state.timingSelectedCell = null;
    }
  }
  if (!state.timingSelectedCell) {
    for (const c of visibleColumns) {
      for (const slot of view.slots) {
        const cell = heatmap.cells.get(`${c.coreKey}|${slot}`);
        if (cell && (!state.timingAnomalyOnly || cell.hasAnomaly)) {
          state.timingSelectedCell = cell.cellKey;
          break;
        }
      }
      if (state.timingSelectedCell) break;
    }
  }

  const timeline = buildTimelineLanes(view, visibleColumns, phaseExplain, compensation);
  const compModelForMismatch = ["distance", "fitted", "hybrid"].includes(state.timingCompModel)
    ? state.timingCompModel
    : "hybrid";
  let firstHybridMismatchTime = null;
  for (const lane of timeline.lanes) {
    for (const slot of lane.expectedSlots) {
      if (getCompStatusByModel(slot, compModelForMismatch) !== "on-time") {
        const t = Number.isFinite(slot.actualTime) ? slot.actualTime : slot.expectedTime;
        if (Number.isFinite(t) && (firstHybridMismatchTime == null || t < firstHybridMismatchTime)) {
          firstHybridMismatchTime = t;
        }
      }
    }
  }
  state.firstHybridMismatchTime = firstHybridMismatchTime;

  renderTimelineSvg(view, timeline);
  renderTimingDrilldown(view, heatmap, phaseExplain, compensation);
  renderFocusedCoreMini(view, timeline);
}

function summarizeEvent(e) {
  if (e.msg === "DataFlow") {
    if (e.Behavior === "FeedIn") {
      return `DataFlow FeedIn data=${e.Data} ${e.From} -> ${e.To}`;
    }
    if (e.Behavior === "Collect") {
      return `DataFlow Collect data=${e.Data} from ${e.From}`;
    }
    return `DataFlow ${e.Behavior} data=${e.Data} ${e.Src} -> ${e.Dst}`;
  }
  if (e.msg === "Inst") {
    return `Inst ${e.OpCode} tile=(${e.X},${e.Y}) id=${e.ID} pred=${e.Pred}`;
  }
  if (e.msg === "Memory") {
    return `Memory ${e.Behavior} tile=(${e.X},${e.Y}) value=${e.Value} addr=${e.Addr}`;
  }
  if (e.msg === "Backpressure") {
    return `Backpressure tile=(${e.X},${e.Y}) dir=${e.DstDir ?? "N/A"} reason=${e.Reason ?? "N/A"} op=${e.OpCode ?? "N/A"} id=${e.ID ?? "N/A"}`;
  }
  return JSON.stringify(e);
}

function applyMeshZoomTransform(transform) {
  meshZoomTransform = transform || d3.zoomIdentity;
  if (sceneRoot) sceneRoot.attr("transform", meshZoomTransform.toString());
}

function bindMeshZoom() {
  if (!meshZoomBehavior) {
    meshZoomBehavior = d3.zoom()
      .scaleExtent([0.4, 8])
      .on("zoom", (event) => {
        applyMeshZoomTransform(event.transform);
      });
  }
  meshZoomBehavior
    .extent([[0, 0], [layout.width, layout.height]])
    .translateExtent([
      [-layout.width * 1.5, -layout.height * 1.5],
      [layout.width * 2.5, layout.height * 2.5],
    ]);
  svg.call(meshZoomBehavior);
  svg.call(meshZoomBehavior.transform, meshZoomTransform);
}

function renderMeshLegend() {
  if (!controls.meshLegend) return;
  const legendItems = [
    ["Send", colors.Send],
    ["Recv", colors.Recv],
    ["FeedIn", colors.FeedIn],
    ["Collect", colors.Collect],
    ["Inst", colors.Inst],
    ["Memory", colors.Memory],
  ];
  controls.meshLegend.innerHTML = legendItems.map(
    ([name, color]) =>
      `<span class="mesh-legend-item"><i class="mesh-legend-dot" style="background:${color}"></i>${name}</span>`,
  ).join("");
}

function drawStaticScene() {
  svg.selectAll("*").remove();
  sceneRoot = svg.append("g").attr("class", "mesh-scene-root");
  applyMeshZoomTransform(meshZoomTransform);
  staticLayer = sceneRoot.append("g");
  dynamicLayer = sceneRoot.append("g");

  const bg = staticLayer.append("rect");
  bg
    .attr("x", 14)
    .attr("y", 14)
    .attr("width", layout.width - 28)
    .attr("height", layout.height - 28)
    .attr("fill", "#fff8e8")
    .attr("stroke", "#ccbfa4")
    .attr("rx", 18);

  const tiles = [];
  for (let y = 0; y <= state.maxY; y += 1) {
    for (let x = 0; x <= state.maxX; x += 1) {
      tiles.push({ x, y });
    }
  }

  const tileGroup = staticLayer.append("g").attr("class", "tile-group");
  tileGroup
    .selectAll("rect")
    .data(tiles)
    .join("rect")
    .attr("class", (d) => `tile tile-${d.x}-${d.y}`)
    .attr("x", (d) => tileRect(d.x, d.y).x)
    .attr("y", (d) => tileRect(d.x, d.y).y)
    .attr("width", layout.tileSize)
    .attr("height", layout.tileSize)
    .attr("rx", 10);

  tileGroup
    .selectAll(".tile-report-heat")
    .data(tiles)
    .join("rect")
    .attr("class", (d) => `tile-report-heat tile-report-heat-${d.x}-${d.y}`)
    .attr("x", (d) => tileRect(d.x, d.y).x)
    .attr("y", (d) => tileRect(d.x, d.y).y)
    .attr("width", layout.tileSize)
    .attr("height", layout.tileSize)
    .attr("rx", 10)
    .attr("opacity", 0)
    .style("display", "none");

  tileGroup
    .selectAll("text")
    .data(tiles)
    .join("text")
    .attr("class", "tile-label")
    .attr("x", (d) => tileRect(d.x, d.y).x + 7)
    .attr("y", (d) => tileRect(d.x, d.y).y + 18)
    .text((d) => `(${d.x},${d.y})`);

  const driverNodes = [];
  for (let i = 0; i <= state.maxX; i += 1) {
    driverNodes.push({ side: "North", idx: i });
    driverNodes.push({ side: "South", idx: i });
  }
  for (let i = 0; i <= state.maxY; i += 1) {
    driverNodes.push({ side: "West", idx: i });
    driverNodes.push({ side: "East", idx: i });
  }

  const drivers = staticLayer.append("g").attr("class", "drivers");
  drivers
    .selectAll("circle")
    .data(driverNodes)
    .join("circle")
    .attr("cx", (d) => endpointPoint({ kind: "driver", side: d.side, idx: d.idx }).x)
    .attr("cy", (d) => endpointPoint({ kind: "driver", side: d.side, idx: d.idx }).y)
    .attr("r", 10)
    .attr("fill", "#f2d6b3")
    .attr("stroke", "#8b7c63")
    .attr("stroke-width", 1.2);

  drivers
    .selectAll("text")
    .data(driverNodes)
    .join("text")
    .attr("class", "driver-label")
    .attr("x", (d) => endpointPoint({ kind: "driver", side: d.side, idx: d.idx }).x + 12)
    .attr("y", (d) => endpointPoint({ kind: "driver", side: d.side, idx: d.idx }).y + 4)
    .text((d) => `${d.side[0]}${d.idx}`);
  applyReportHeatOverlay();
  renderMeshLegend();
  bindMeshZoom();
}

function drawLink(type, srcPoint, dstPoint, payload = null) {
  const path = d3.path();
  path.moveTo(srcPoint.x, srcPoint.y);
  const dx = dstPoint.x - srcPoint.x;
  const dy = dstPoint.y - srcPoint.y;
  const curve = Math.abs(dx) > Math.abs(dy) ? 0.25 : -0.25;
  path.bezierCurveTo(
    srcPoint.x + dx * 0.35,
    srcPoint.y + dy * curve,
    srcPoint.x + dx * 0.65,
    dstPoint.y - dy * curve,
    dstPoint.x,
    dstPoint.y,
  );

  const link = dynamicLayer
    .append("path")
    .attr("class", "event-link")
    .attr("d", path.toString())
    .attr("stroke", colors[type] || "#555")
    .attr("stroke-opacity", 0.78);

  const dataText = payload?.dataText == null ? "" : String(payload.dataText);
  if (payload?.drawDataLabel && dataText) {
    const shortData = shortText(dataText, 12);
    let anchorX = srcPoint.x + dx * 0.58;
    let anchorY = srcPoint.y + dy * 0.58;
    try {
      const node = link.node();
      if (node) {
        const total = node.getTotalLength();
        if (Number.isFinite(total) && total > 0) {
          const p = node.getPointAtLength(total * 0.58);
          anchorX = p.x;
          anchorY = p.y;
        }
      }
    } catch (_) {
      // Fall back to linear interpolation point when path metrics unavailable.
    }
    const tag = dynamicLayer.append("g")
      .attr("class", "flow-data-tag")
      .attr("transform", `translate(${anchorX},${anchorY})`);
    const text = tag.append("text")
      .attr("class", "flow-data-text")
      .attr("text-anchor", "middle")
      .attr("dominant-baseline", "middle")
      .text(shortData);
    const box = text.node()?.getBBox();
    if (box) {
      tag.insert("rect", "text")
        .attr("class", "flow-data-bg")
        .attr("x", box.x - 3)
        .attr("y", box.y - 1)
        .attr("width", box.width + 6)
        .attr("height", box.height + 2)
        .attr("rx", 4);
    }
    tag.append("title").text(`data=${dataText}`);
  }

  const pulse = dynamicLayer
    .append("circle")
    .attr("class", "pulse")
    .attr("cx", srcPoint.x)
    .attr("cy", srcPoint.y)
    .attr("fill", colors[type] || "#444")
    .attr("opacity", 0.95);

  pulse
    .transition()
    .duration(Math.max(220, state.speedMs - 160))
    .ease(d3.easeCubicInOut)
    .attr("cx", dstPoint.x)
    .attr("cy", dstPoint.y)
    .attr("opacity", 0.2)
    .remove();
}

function applyTileActivity(activeTiles) {
  staticLayer.selectAll(".tile").classed("active", false);
  for (const key of activeTiles) {
    const [x, y] = key.split(",").map(Number);
    staticLayer.select(`.tile-${x}-${y}`).classed("active", true);
  }
  staticLayer.selectAll(".tile-label").style("display", state.showLabels ? null : "none");
}

function shortText(value, maxLen = 16) {
  const s = String(value ?? "").trim();
  if (!s) return "";
  return s.length <= maxLen ? s : `${s.slice(0, maxLen - 1)}~`;
}

function summarizeTokens(tokens, prefix, maxItems = 3) {
  if (!Array.isArray(tokens) || tokens.length === 0) return null;
  const counts = new Map();
  for (const token of tokens) {
    const key = String(token || "N/A");
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  const sorted = [...counts.entries()]
    .sort((a, b) => {
      if (b[1] !== a[1]) return b[1] - a[1];
      return a[0].localeCompare(b[0]);
    });
  const picked = sorted.slice(0, maxItems).map(([k, v]) => (v > 1 ? `${k}*${v}` : k));
  const remain = sorted.length - maxItems;
  const suffix = remain > 0 ? `,+${remain}` : "";
  return `${prefix}:${picked.join(",")}${suffix}`;
}

function summarizeData(values, prefix, maxItems = 4) {
  if (!Array.isArray(values) || values.length === 0) return null;
  const picked = values.slice(0, maxItems).map((v) => shortText(v, 8));
  const suffix = values.length > maxItems ? ",..." : "";
  return `${prefix}:${picked.join(",")}${suffix}`;
}

function drawTileBadges(timeEvents) {
  const byTile = new Map();
  const ensure = (key) => {
    if (!byTile.has(key)) {
      byTile.set(key, {
        op: [],
        mem: [],
        txData: [],
        rxData: [],
        details: [],
      });
    }
    return byTile.get(key);
  };

  for (const e of timeEvents) {
    if (e.msg === "Inst" && state.showInst && Number.isFinite(Number(e.X)) && Number.isFinite(Number(e.Y))) {
      const k = tileKey(Number(e.X), Number(e.Y));
      const rec = ensure(k);
      const op = shortText(e.OpCode || "Inst", 10);
      rec.op.push(op || "Inst");
      rec.details.push(`Inst#${e.ID ?? "?"} ${e.OpCode ?? "N/A"} pred=${e.Pred ?? "N/A"}`);
      continue;
    }

    if (e.msg === "Memory" && state.showMemory && Number.isFinite(Number(e.X)) && Number.isFinite(Number(e.Y))) {
      const k = tileKey(Number(e.X), Number(e.Y));
      const rec = ensure(k);
      const behavior = String(e.Behavior || "Memory");
      const memTag = behavior === "LoadDirect"
        ? `LD(${shortText(e.Value, 6)})`
        : (behavior === "StoreDirect"
          ? `ST(${shortText(e.Value, 6)})`
          : shortText(behavior, 12));
      rec.mem.push(memTag);
      rec.details.push(`Memory ${behavior} value=${e.Value ?? "N/A"} addr=${e.Addr ?? "N/A"}`);
      continue;
    }

    if (e.msg === "DataFlow" && state.showDataFlow) {
      const dataValue = e.Data;
      if (e.Behavior === "Send") {
        const src = parseEndpoint(e.Src);
        if (src?.kind === "tilePort") {
          const rec = ensure(tileKey(src.x, src.y));
          rec.txData.push(dataValue);
          rec.details.push(`TX ${dataValue} ${e.Src} -> ${e.Dst}`);
        }
      } else if (e.Behavior === "Recv") {
        const dst = parseEndpoint(e.Dst);
        if (dst?.kind === "tilePort") {
          const rec = ensure(tileKey(dst.x, dst.y));
          rec.rxData.push(dataValue);
          rec.details.push(`RX ${dataValue} ${e.Src} -> ${e.Dst}`);
        }
      } else if (e.Behavior === "FeedIn") {
        const dst = parseEndpoint(e.To);
        if (dst?.kind === "tilePort") {
          const rec = ensure(tileKey(dst.x, dst.y));
          rec.rxData.push(dataValue);
          rec.details.push(`FeedIn ${dataValue} ${e.From} -> ${e.To}`);
        }
      } else if (e.Behavior === "Collect") {
        const src = parseEndpoint(e.From);
        if (src?.kind === "tilePort") {
          const rec = ensure(tileKey(src.x, src.y));
          rec.txData.push(dataValue);
          rec.details.push(`Collect ${dataValue} ${e.From} -> ${e.To || e.Dst || "Driver"}`);
        }
      }
    }
  }

  for (const [k, rec] of byTile.entries()) {
    const [x, y] = k.split(",").map(Number);
    const r = tileRect(x, y);
    const lineHeight = clamp(Math.round(layout.tileSize * 0.12), 9, 13);
    const fontSize = clamp(Math.round(layout.tileSize * 0.1), 7, 11);
    const innerWidth = Math.max(20, r.w - 8);
    const approxCharWidth = fontSize * 0.6;
    const maxCharsPerLine = Math.max(4, Math.floor(innerWidth / approxCharWidth));
    const textTop = r.y + 24;
    const maxTextHeight = Math.max(8, r.h - 30);
    const maxLines = Math.max(1, Math.floor(maxTextHeight / lineHeight));

    const lines = [];
    const lineOps = [summarizeTokens(rec.op, "OP", 2), summarizeTokens(rec.mem, "MEM", 1)]
      .filter(Boolean)
      .join(" | ");
    const lineFlow = [summarizeData(rec.rxData, "RX", 2), summarizeData(rec.txData, "TX", 2)]
      .filter(Boolean)
      .join(" | ");
    // Prioritize flow values so data is still visible when space is tight.
    if (lineFlow) lines.push(lineFlow);
    if (lineOps) lines.push(lineOps);
    if (lines.length === 0) continue;

    const shown = lines.slice(0, maxLines).map((line) => shortText(line, maxCharsPerLine));
    if (lines.length > maxLines) {
      shown[maxLines - 1] = `${shortText(shown[maxLines - 1], Math.max(4, maxCharsPerLine - 3))}...`;
    }
    const bgHeight = shown.length * lineHeight + 8;
    const g = dynamicLayer.append("g")
      .attr("class", "tile-overlay")
      .attr("transform", `translate(${r.x + 4},${textTop})`);
    g.append("rect")
      .attr("class", "tile-overlay-card")
      .attr("width", innerWidth)
      .attr("height", bgHeight)
      .attr("rx", 4);
    const text = g.append("text")
      .attr("class", "tile-overlay-text")
      .style("font-size", `${fontSize}px`)
      .attr("x", 4)
      .attr("y", lineHeight - 1);
    shown.forEach((line, idx) => {
      text.append("tspan")
        .attr("x", 4)
        .attr("dy", idx === 0 ? 0 : lineHeight)
        .text(line);
    });
    g.append("title").text(
      [
        `tile=(${x},${y})`,
        ...lines,
        ...rec.details.slice(0, 12),
      ].join("\n"),
    );
  }
}

function renderCycleDetails(events, t) {
  const counts = {
    DataFlow: 0,
    Inst: 0,
    Memory: 0,
  };
  for (const e of events) {
    if (e.msg === "DataFlow") counts.DataFlow += 1;
    if (e.msg === "Inst") counts.Inst += 1;
    if (e.msg === "Memory") counts.Memory += 1;
  }
  controls.statsLine.textContent =
    `Cycle ${t} | DataFlow=${counts.DataFlow} | Inst=${counts.Inst} | Memory=${counts.Memory}`;
  controls.eventDump.textContent = events.map(summarizeEvent).join("\n");
}

function renderTime(t) {
  const cycle = clamp(normalizeCycleTime(t, state.currentTime), state.minTime, state.maxTime);
  state.currentTime = cycle;
  controls.timeLabel.textContent = `T=${cycle}`;
  controls.timeSlider.value = String(cycle);
  dynamicLayer.selectAll("*").remove();

  const events = state.byTime.get(cycle) || [];
  const activeTiles = new Set();
  const linkLabelSeen = new Set();

  for (const e of events) {
    if (e.msg === "DataFlow" && state.showDataFlow) {
      let src = null;
      let dst = null;
      let type = e.Behavior;
      if (e.Behavior === "FeedIn") {
        src = parseEndpoint(e.From);
        dst = parseEndpoint(e.To);
      } else if (e.Behavior === "Collect") {
        src = parseEndpoint(e.From);
      } else {
        src = parseEndpoint(e.Src);
        dst = parseEndpoint(e.Dst);
      }
      const srcPoint = endpointPoint(src);
      const dstPoint = endpointPoint(dst);
      if (srcPoint && dstPoint) {
        const dataText = e.Data == null ? "" : String(e.Data);
        const labelKey = `${src?.raw || srcPoint.tile || "?"}|${dst?.raw || dstPoint.tile || "?"}|${dataText}`;
        const drawDataLabel = dataText && !linkLabelSeen.has(labelKey);
        if (drawDataLabel) linkLabelSeen.add(labelKey);
        drawLink(type, srcPoint, dstPoint, { dataText, drawDataLabel });
      } else if (srcPoint) {
        dynamicLayer
          .append("circle")
          .attr("class", "pulse")
          .attr("cx", srcPoint.x)
          .attr("cy", srcPoint.y)
          .attr("fill", colors[type] || "#333")
          .attr("opacity", 0.9)
          .transition()
          .duration(Math.max(220, state.speedMs - 200))
          .attr("r", 9)
          .attr("opacity", 0.15)
          .remove();
      }
      if (srcPoint && srcPoint.tile) activeTiles.add(srcPoint.tile);
      if (dstPoint && dstPoint.tile) activeTiles.add(dstPoint.tile);
    }
    if (e.msg === "Inst" && state.showInst) {
      activeTiles.add(tileKey(e.X, e.Y));
    }
    if (e.msg === "Memory" && state.showMemory) {
      activeTiles.add(tileKey(e.X, e.Y));
    }
  }

  applyTileActivity(activeTiles);
  drawTileBadges(events);
  // Keep link arrows above tile overlay cards.
  dynamicLayer.selectAll(".event-link").raise();
  // Keep transfer data labels/pulses above tile cards for readability.
  dynamicLayer.selectAll(".flow-data-tag").raise();
  dynamicLayer.selectAll(".pulse").raise();
  renderCycleDetails(events, cycle);
}

function stopPlayback() {
  if (state.timer) {
    clearTimeout(state.timer);
    state.timer = null;
  }
  controls.playBtn.textContent = "Play";
}

function playbackTick() {
  if (!state.timer) return;
  const next = nextIndexedTime(state.currentTime, +1);
  if (next <= state.currentTime) {
    stopPlayback();
    return;
  }
  renderTime(next);
  state.timer = setTimeout(playbackTick, state.speedMs);
}

function playOrPause() {
  if (state.timer) {
    stopPlayback();
    return;
  }
  if (state.currentTime >= state.maxTime) renderTime(state.maxTime);
  controls.playBtn.textContent = "Pause";
  state.timer = setTimeout(playbackTick, state.speedMs);
}

function initControls() {
  controls.playBtn.addEventListener("click", playOrPause);
  controls.stepBackBtn.addEventListener("click", () => {
    if (state.stepLock) return;
    state.stepLock = true;
    stopPlayback();
    try {
      renderTime(nextIndexedTime(state.currentTime, -1));
    } finally {
      state.stepLock = false;
    }
  });
  controls.stepFwdBtn.addEventListener("click", () => {
    if (state.stepLock) return;
    state.stepLock = true;
    stopPlayback();
    try {
      renderTime(nextIndexedTime(state.currentTime, +1));
    } finally {
      state.stepLock = false;
    }
  });
  controls.timeSlider.addEventListener("input", (e) => {
    const wasPlaying = Boolean(state.timer);
    stopPlayback();
    const nextTime = Number(e.target.value);
    renderTime(nextTime);
    if (wasPlaying) {
      playOrPause();
    }
  });
  controls.speedSelect.addEventListener("change", (e) => {
    state.speedMs = Number(e.target.value);
    if (state.timer) {
      stopPlayback();
      playOrPause();
    }
  });
  controls.showDataFlow.addEventListener("change", (e) => {
    state.showDataFlow = Boolean(e.target.checked);
    renderTime(state.currentTime);
  });
  controls.showInst.addEventListener("change", (e) => {
    state.showInst = Boolean(e.target.checked);
    renderTime(state.currentTime);
  });
  controls.showMemory.addEventListener("change", (e) => {
    state.showMemory = Boolean(e.target.checked);
    renderTime(state.currentTime);
  });
  controls.showLabels.addEventListener("change", (e) => {
    state.showLabels = Boolean(e.target.checked);
    renderTime(state.currentTime);
  });
  if (controls.timingAnomalyOnly) {
    controls.timingAnomalyOnly.checked = state.timingAnomalyOnly;
    controls.timingAnomalyOnly.addEventListener("change", (e) => {
      state.timingAnomalyOnly = Boolean(e.target.checked);
      renderTimingView();
    });
  }
  if (controls.timingShowPhaseExplain) {
    controls.timingShowPhaseExplain.checked = state.showPhaseExplain;
    controls.timingShowPhaseExplain.addEventListener("change", (e) => {
      state.showPhaseExplain = Boolean(e.target.checked);
      renderTimingView();
    });
  }
  if (controls.timingBoundaryOnly) {
    controls.timingBoundaryOnly.checked = state.timingBoundaryOnly;
    controls.timingBoundaryOnly.addEventListener("change", (e) => {
      state.timingBoundaryOnly = Boolean(e.target.checked);
      state.timingSelectedCell = null;
      renderTimingView();
    });
  }
  if (controls.timingCoreFocus) {
    controls.timingCoreFocus.addEventListener("change", (e) => {
      const value = String(e.target.value || "");
      state.timingFocusedCoreKey = value || null;
      state.timingSelectedCell = null;
      renderTimingView();
    });
  }
  if (controls.timingIoWaveAll) {
    controls.timingIoWaveAll.addEventListener("change", (e) => {
      const checked = Boolean(e.target.checked);
      state.timingIoWaveExpandAll = checked;
      if (checked) {
        state.timingIoWaveExpandedCoreKeys = new Set((state.timingRows || []).map((c) => c.coreKey));
      }
      renderTimingView();
    });
  }
  if (controls.timingIoWaveCore) {
    controls.timingIoWaveCore.addEventListener("change", (e) => {
      const selectedKeys = new Set(
        [...e.target.selectedOptions]
          .map((opt) => String(opt.value || ""))
          .filter(Boolean),
      );
      state.timingIoWaveExpandedCoreKeys = selectedKeys;
      const total = (state.timingRows || []).length;
      state.timingIoWaveExpandAll = total > 0 && selectedKeys.size >= total;
      renderTimingView();
    });
  }
  if (controls.timingBaselineView) {
    controls.timingBaselineView.value = state.timingBaselineView;
    controls.timingBaselineView.addEventListener("change", (e) => {
      state.timingBaselineView = String(e.target.value || "strict");
      renderTimingView();
    });
  }
  if (controls.timingCompModel) {
    controls.timingCompModel.value = state.timingCompModel;
    controls.timingCompModel.addEventListener("change", (e) => {
      state.timingCompModel = String(e.target.value || "hybrid");
      renderTimingView();
    });
  }
  if (controls.timingExportPng) {
    controls.timingExportPng.addEventListener("click", exportTimelinePng);
  }
  if (controls.timingJumpFirstMismatch) {
    controls.timingJumpFirstMismatch.addEventListener("click", () => {
      if (state.firstHybridMismatchTime == null || !Number.isFinite(state.firstHybridMismatchTime)) return;
      const half = Math.floor((Number(state.timingWindowSize) || 60) / 2);
      state.timingWindowStart = Math.max(0, state.firstHybridMismatchTime - half);
      renderTimingView();
    });
  }
  if (controls.timingWindowStart) {
    controls.timingWindowStart.addEventListener("input", (e) => {
      state.timingWindowStart = Number(e.target.value);
      renderTimingView();
    });
  }
  if (controls.timingWindowSize) {
    controls.timingWindowSize.addEventListener("input", (e) => {
      state.timingWindowSize = Number(e.target.value);
      renderTimingView();
    });
  }
  if (controls.timingZoomY) {
    controls.timingZoomY.addEventListener("input", (e) => {
      state.timingZoomY = clamp(Number(e.target.value) / 100, 0.6, 4);
      renderTimingView();
    });
  }
  if (controls.timingResetZoom) {
    controls.timingResetZoom.addEventListener("click", () => {
      state.timingZoomX = 1;
      state.timingZoomY = 1;
      const fullMin = state.timingViewport?.fullMin ?? state.minTime;
      const fullMax = state.timingViewport?.fullMax ?? state.maxTime;
      const fullSpan = Math.max(1, fullMax - fullMin + 1);
      state.timingWindowSize = Math.min(120, fullSpan);
      state.timingWindowStart = fullMin;
      renderTimingView();
    });
  }
  if (controls.timingGrid) {
    controls.timingGrid.addEventListener("wheel", handleTimelineCtrlWheelZoom, { passive: false });
    controls.timingGrid.addEventListener("click", (e) => {
      const label = e.target.closest("[data-core-key]");
      if (label) {
        if (timingCoreLabelClickTimer) clearTimeout(timingCoreLabelClickTimer);
        const key = label.getAttribute("data-core-key");
        timingCoreLabelClickTimer = setTimeout(() => {
          timingCoreLabelClickTimer = null;
          state.timingFocusedCoreKey = state.timingFocusedCoreKey === key ? null : key;
          state.timingSelectedCell = null;
          renderTimingView();
        }, 220);
        return;
      }
      const btn = e.target.closest("[data-timing-cell]");
      if (!btn) return;
      state.timingSelectedCell = btn.getAttribute("data-timing-cell");
      renderTimingView();
    });
    controls.timingGrid.addEventListener("dblclick", (e) => {
      const label = e.target.closest("[data-core-key]");
      if (!label) return;
      if (timingCoreLabelClickTimer) {
        clearTimeout(timingCoreLabelClickTimer);
        timingCoreLabelClickTimer = null;
      }
      const key = label.getAttribute("data-core-key");
      if (!key) return;
      const expanded = new Set(state.timingIoWaveExpandedCoreKeys || []);
      if (expanded.has(key)) {
        expanded.delete(key);
      } else {
        expanded.add(key);
      }
      state.timingIoWaveExpandedCoreKeys = expanded;
      const total = (state.timingRows || []).length;
      state.timingIoWaveExpandAll = total > 0 && expanded.size >= total;
      renderTimingView();
    });
  }
  controls.fileInput.addEventListener("change", async (e) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    loadTrace(text);
  });
  controls.yamlInput.addEventListener("change", async (e) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    loadProgramYaml(text);
  });
  if (controls.reportInput) {
    controls.reportInput.addEventListener("change", async (e) => {
      const file = e.target.files?.[0];
      if (!file) return;
      const text = await file.text();
      loadReport(text);
    });
  }
}

function loadTrace(text) {
  stopPlayback();
  const events = parseJsonLines(text);
  state.events = events;
  state.coreIoWaveByTime = buildCoreIoWaveByTime(events);
  state.timingSelectedCell = null;
  state.timingFocusedCoreKey = null;
  state.timingIoWaveExpandAll = false;
  state.timingIoWaveExpandedCoreKeys = new Set();
  state.timingWindowStart = 0;
  state.timingWindowSize = 120;
  state.timingZoomX = 1;
  state.timingZoomY = 1;
  state.timingViewport = null;
  meshZoomTransform = d3.zoomIdentity;
  const bounds = resolveMeshBounds(events);
  state.maxX = bounds.maxX;
  state.maxY = bounds.maxY;
  const index = indexByTime(events);
  state.byTime = index.byTime;
  state.timeKeys = index.sortedTimes;
  state.minTime = index.minTime;
  state.maxTime = index.maxTime;

  controls.timeSlider.min = String(state.minTime);
  controls.timeSlider.max = String(state.maxTime);
  controls.timeSlider.value = String(state.minTime);

  applyAdaptiveLayout();
  drawStaticScene();
  renderTime(state.minTime);
  renderReportView();
  renderTimingView();
}

function loadProgramYaml(text) {
  try {
    state.programSpec = parseProgramYaml(text);
    state.yamlGridBounds = boundsFromProgramSpec(state.programSpec);
    state.timingSelectedCell = null;
    state.timingFocusedCoreKey = null;
    state.timingIoWaveExpandAll = false;
    state.timingIoWaveExpandedCoreKeys = new Set();
    state.timingWindowStart = 0;
    state.timingZoomX = 1;
    state.timingZoomY = 1;
    state.timingViewport = null;
    meshZoomTransform = d3.zoomIdentity;
    const bounds = state.yamlGridBounds || inferBounds(state.events);
    state.maxX = bounds.maxX;
    state.maxY = bounds.maxY;
    applyAdaptiveLayout();
    drawStaticScene();
    if (state.events.length > 0) {
      renderTime(clamp(state.currentTime, state.minTime, state.maxTime));
    }
    renderReportView();
    renderTimingView();
  } catch (err) {
    state.programSpec = null;
    state.yamlGridBounds = null;
    state.timingReady = false;
    state.timingFocusedCoreKey = null;
    state.timingIoWaveExpandAll = false;
    state.timingIoWaveExpandedCoreKeys = new Set();
    controls.timingSummary.textContent = `Program YAML parse error: ${err.message}`;
    controls.timingGrid.innerHTML = "";
    if (controls.timingCoreFocus) {
      controls.timingCoreFocus.innerHTML = "<option value=\"\">All cores</option>";
      controls.timingCoreFocus.value = "";
    }
    if (controls.timingIoWaveCore) {
      controls.timingIoWaveCore.innerHTML = "";
    }
    if (controls.timingIoWaveAll) {
      controls.timingIoWaveAll.checked = false;
    }
    if (controls.timingDrilldown) {
      controls.timingDrilldown.innerHTML =
        "<div class=\"timing-drill-empty\">Program YAML parse failed. Fix YAML and reload.</div>";
    }
    if (controls.timingCoreMini) {
      controls.timingCoreMini.innerHTML =
        "<div class=\"timing-core-mini-empty\">Focus one core to inspect local trace details.</div>";
    }
    renderReportView();
  }
}

let resizeTimer = null;

function handleResize() {
  if (state.events.length === 0 && !state.programSpec) return;
  if (resizeTimer) clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    applyAdaptiveLayout();
    drawStaticScene();
    if (state.events.length > 0) renderTime(state.currentTime);
  }, 120);
}

async function boot() {
  initControls();
  applyAdaptiveLayout();
  renderReportView();
  window.addEventListener("resize", handleResize);

  // Default behavior: load ../gemm.json.log when served from repo root.
  try {
    const resp = await fetch("../gemm.json.log");
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const text = await resp.text();
    loadTrace(text);
  } catch (_) {
    controls.statsLine.textContent = "Default log not loaded. Use the file picker.";
    controls.eventDump.textContent = "";
  }

  try {
    const yamlResp = await fetch("../gemm.yaml");
    if (!yamlResp.ok) throw new Error(`HTTP ${yamlResp.status}`);
    const yamlText = await yamlResp.text();
    loadProgramYaml(yamlText);
  } catch (_) {
    renderTimingView();
  }
}

boot();
