const state = {
  events: [],
  byTime: new Map(),
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
};

const layout = {
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
let staticLayer;
let dynamicLayer;

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
  statsLine: document.getElementById("statsLine"),
  eventDump: document.getElementById("eventDump"),
};

function tileKey(x, y) {
  return `${x},${y}`;
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

function parseJsonLines(text) {
  const lines = text.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
  const rows = [];
  for (const line of lines) {
    try {
      const obj = JSON.parse(line);
      if (obj && typeof obj.Time === "number" && Number.isFinite(obj.Time)) {
        obj.Time = Math.round(obj.Time);
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
  return { byTime, minTime, maxTime };
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
  return JSON.stringify(e);
}

function drawStaticScene() {
  svg.selectAll("*").remove();
  staticLayer = svg.append("g");
  dynamicLayer = svg.append("g");

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

  const legend = staticLayer.append("g").attr("transform", "translate(28, 34)");
  const legendItems = [
    ["Send", colors.Send],
    ["Recv", colors.Recv],
    ["FeedIn", colors.FeedIn],
    ["Collect", colors.Collect],
    ["Inst", colors.Inst],
    ["Memory", colors.Memory],
  ];
  legend
    .selectAll("circle")
    .data(legendItems)
    .join("circle")
    .attr("cx", (_d, i) => i * 112)
    .attr("cy", 0)
    .attr("r", 5)
    .attr("fill", (d) => d[1]);
  legend
    .selectAll("text")
    .data(legendItems)
    .join("text")
    .attr("class", "legend-text")
    .attr("x", (_d, i) => i * 112 + 9)
    .attr("y", 4)
    .text((d) => d[0]);
}

function drawLink(type, srcPoint, dstPoint) {
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

  dynamicLayer
    .append("path")
    .attr("class", "event-link")
    .attr("d", path.toString())
    .attr("stroke", colors[type] || "#555")
    .attr("stroke-opacity", 0.78);

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

function drawTileBadges(timeEvents) {
  const instCounts = new Map();
  const memCounts = new Map();

  for (const e of timeEvents) {
    if (e.msg === "Inst" && state.showInst) {
      const k = tileKey(e.X, e.Y);
      instCounts.set(k, (instCounts.get(k) || 0) + 1);
    }
    if (e.msg === "Memory" && state.showMemory) {
      const k = tileKey(e.X, e.Y);
      memCounts.set(k, (memCounts.get(k) || 0) + 1);
    }
  }

  for (const [k, count] of instCounts.entries()) {
    const [x, y] = k.split(",").map(Number);
    const r = tileRect(x, y);
    dynamicLayer
      .append("circle")
      .attr("class", "inst-badge")
      .attr("cx", r.x + 16)
      .attr("cy", r.y + 16)
      .attr("r", 10)
      .attr("fill", colors.Inst)
      .attr("opacity", 0.9);
    dynamicLayer
      .append("text")
      .attr("x", r.x + 12)
      .attr("y", r.y + 20)
      .attr("fill", "#fff")
      .attr("font-size", 11)
      .text(`${count}`);
  }

  for (const [k, count] of memCounts.entries()) {
    const [x, y] = k.split(",").map(Number);
    const r = tileRect(x, y);
    dynamicLayer
      .append("rect")
      .attr("class", "memory-badge")
      .attr("x", r.x + r.w - 23)
      .attr("y", r.y + r.h - 23)
      .attr("width", 16)
      .attr("height", 16)
      .attr("rx", 3)
      .attr("fill", colors.Memory)
      .attr("opacity", 0.9);
    dynamicLayer
      .append("text")
      .attr("x", r.x + r.w - 20)
      .attr("y", r.y + r.h - 11)
      .attr("fill", "#fff")
      .attr("font-size", 11)
      .text(`${count}`);
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
  state.currentTime = t;
  controls.timeLabel.textContent = `T=${t}`;
  controls.timeSlider.value = String(t);
  dynamicLayer.selectAll("*").remove();

  const events = state.byTime.get(t) || [];
  const activeTiles = new Set();

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
        drawLink(type, srcPoint, dstPoint);
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
  renderCycleDetails(events, t);
}

function stopPlayback() {
  if (state.timer) {
    clearInterval(state.timer);
    state.timer = null;
  }
  controls.playBtn.textContent = "Play";
}

function playOrPause() {
  if (state.timer) {
    stopPlayback();
    return;
  }
  controls.playBtn.textContent = "Pause";
  state.timer = setInterval(() => {
    if (state.currentTime >= state.maxTime) {
      stopPlayback();
      return;
    }
    renderTime(state.currentTime + 1);
  }, state.speedMs);
}

function initControls() {
  controls.playBtn.addEventListener("click", playOrPause);
  controls.stepBackBtn.addEventListener("click", () => {
    stopPlayback();
    renderTime(Math.max(state.minTime, state.currentTime - 1));
  });
  controls.stepFwdBtn.addEventListener("click", () => {
    stopPlayback();
    renderTime(Math.min(state.maxTime, state.currentTime + 1));
  });
  controls.timeSlider.addEventListener("input", (e) => {
    stopPlayback();
    renderTime(Number(e.target.value));
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
  controls.fileInput.addEventListener("change", async (e) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    loadTrace(text);
  });
}

function loadTrace(text) {
  stopPlayback();
  const events = parseJsonLines(text);
  state.events = events;
  const bounds = inferBounds(events);
  state.maxX = bounds.maxX;
  state.maxY = bounds.maxY;
  const index = indexByTime(events);
  state.byTime = index.byTime;
  state.minTime = index.minTime;
  state.maxTime = index.maxTime;

  controls.timeSlider.min = String(state.minTime);
  controls.timeSlider.max = String(state.maxTime);
  controls.timeSlider.value = String(state.minTime);

  drawStaticScene();
  renderTime(state.minTime);
}

async function boot() {
  initControls();

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
}

boot();
