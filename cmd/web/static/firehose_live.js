/**
 * firehose_live.js
 *
 * Real-time Firehose WebSocket client for Ergs.
 * - Discovers initial blocks rendered server-side.
 * - Establishes a WebSocket connection to /api/firehose/ws (with ?since= cursor).
 * - Streams new blocks (single or batched) and prepends them to the list.
 * - Maintains a high-water mark cursor to minimize snapshot size on reconnect.
 * - Exponential backoff reconnect with cap.
 * - DOM growth limiting (oldest blocks pruned beyond a configurable cap).
 * - Status badge updates: LIVE / Connecting… / Reconnecting… / Error.
 *
 * Assumptions:
 * - Page contains a container: .firehose-blocks
 * - Each initial server block has data attributes:
 *     data-block-id
 *     data-block-source
 *     data-created-at (RFC3339)
 * - A status badge element exists: #firehose-live-status
 *
 * Configuration:
 *   Set a global (optional) before this script loads:
 *     window.ERG_FIREHOSE_CONFIG = {
 *       maxBlocks: 1500,           // default 1000
 *       reconnectBaseMs: 1000,     // default 1000
 *       reconnectMaxMs: 30000,     // default 30000
 *       wsPath: "/api/firehose/ws" // override if behind proxy prefix
 *     }
 *
 * No backward compatibility logic is kept for pagination UI; this is a pure stream model.
 */

(() => {
  "use strict";

  if (window.__ERG_FIREHOSE_LIVE_INITIALIZED__) return;
  window.__ERG_FIREHOSE_LIVE_INITIALIZED__ = true;

  const cfg = Object.assign(
    {
      maxBlocks: 100,
      reconnectBaseMs: 1000,
      reconnectMaxMs: 30000,
      wsPath: "/api/firehose/ws",
    },
    window.ERG_FIREHOSE_CONFIG || {},
  );

  // Acquire (or wait for) the firehose blocks container. In some navigations the
  // script can execute before the HTML for the list is inserted (or restored
  // from a streaming/templating step). Previously we aborted silently; now we
  // wait up to ~10s so the page can hydrate without requiring a manual refresh.
  let blocksRoot = document.querySelector(".firehose-blocks");
  let pendingInit = false;

  if (!blocksRoot) {
    pendingInit = true;
    console.log(
      "[firehose] .firehose-blocks not found yet; waiting for DOM insertion",
    );
    let attempts = 0;
    const maxAttempts = 50; // 50 * 200ms = 10s
    (function retry() {
      blocksRoot = document.querySelector(".firehose-blocks");
      if (blocksRoot) {
        console.log(
          "[firehose] found .firehose-blocks after %d attempts",
          attempts,
        );
        pendingInit = false;
        // Proceed now that container exists
        scanInitial();
        connect();
      } else if (++attempts < maxAttempts) {
        return setTimeout(retry, 200);
      } else {
        console.error(
          "[firehose] abort: .firehose-blocks never appeared after waiting; creating fallback container",
        );
        // Create a fallback container so live updates can still render
        const fallback = document.createElement("div");
        fallback.className = "firehose-blocks";
        const anchor =
          document.querySelector(".firehose-header") || document.body;
        if (anchor && anchor.parentNode) {
          anchor.parentNode.insertBefore(
            fallback,
            anchor.nextSibling ? anchor.nextSibling : null,
          );
        } else {
          document.body.appendChild(fallback);
        }
        blocksRoot = fallback;
        console.log("[firehose] inserted fallback .firehose-blocks container");
        pendingInit = false;
        scanInitial();
        connect();
      }
    })();
  }

  const statusEl = document.getElementById("firehose-live-status");

  const seen = new Set(); // key = source + ":" + id
  let cursor = null; // Most recent created_at Date
  let backoff = cfg.reconnectBaseMs;
  let ws = null;
  let reconnectTimer = null;
  let connectionMode = null; // "push" | "poll" | null (reported by server in init)
  // DOM growth control additions
  let insertCount = 0;
  const PRUNE_INSERT_INTERVAL = 100; // Every N successful inserts, run a fast prune
  const SAFETY_PRUNE_INTERVAL_MS = 60000; // Full scan safety prune every 60s

  // --- Utility Functions ----------------------------------------------------

  function setStatus(state, label) {
    if (!statusEl) return;
    statusEl.dataset.state = state;
    // Keep an accessible label but do not render text inside the dot.
    if (label) {
      statusEl.setAttribute("aria-label", label);
    } else {
      statusEl.setAttribute("aria-label", state);
    }
    // Intentionally do not modify textContent to keep the indicator purely visual (dot).
  }

  function parseRFC3339(s) {
    if (!s) return null;
    const d = new Date(s);
    return isNaN(d) ? null : d;
  }

  function escapeHtml(s) {
    return String(s).replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
  }

  function updateCursorIfNewer(ts) {
    const d = typeof ts === "string" ? parseRFC3339(ts) : ts;
    if (!d) return;
    if (!cursor || d > cursor) cursor = d;
  }

  function blockKey(source, id) {
    return source + ":" + id;
  }

  function trimDomIfNeeded(forceFullScan = false) {
    if (!blocksRoot) return;
    const wrappers = blocksRoot.querySelectorAll(".firehose-block-wrapper");
    if (wrappers.length <= cfg.maxBlocks) return;
    // Fast path: while loop pop from end until at cap.
    let excess = wrappers.length - cfg.maxBlocks;
    if (!forceFullScan) {
      while (excess > 0) {
        const w = blocksRoot.lastElementChild;
        if (!w || !w.classList.contains("firehose-block-wrapper")) break;
        blocksRoot.removeChild(w);
        excess--;
      }
      return;
    }
    // Full scan (safety mode): remove any surplus anywhere (defensive if external nodes injected).
    let removed = 0;
    for (
      let i = wrappers.length - 1;
      i >= 0 && wrappers.length - removed > cfg.maxBlocks;
      i--
    ) {
      const w = wrappers[i];
      if (w && w.parentNode === blocksRoot) {
        blocksRoot.removeChild(w);
        removed++;
      }
    }
  }

  function createBlockElement(b) {
    const source =
      b.source ||
      (b.metadata && (b.metadata.datasource || b.metadata.source)) ||
      "unknown";
    const rawCreated = b.created_at || new Date().toISOString();

    // Format for display: YYYY-MM-DD HH:MM (local time), keep rawCreated for data attribute & cursor logic
    function formatShort(ts) {
      const d = new Date(ts);
      if (isNaN(d)) return ts;
      const pad = (n) => (n < 10 ? "0" + n : "" + n);
      return (
        d.getFullYear() +
        "-" +
        pad(d.getMonth() + 1) +
        "-" +
        pad(d.getDate()) +
        " " +
        pad(d.getHours()) +
        ":" +
        pad(d.getMinutes())
      );
    }
    const createdDisplay = formatShort(rawCreated);

    const id = b.id || "";
    const key = blockKey(source, id);

    if (!id || seen.has(key)) return null;
    seen.add(key);

    // Build wrapper
    const wrapper = document.createElement("div");
    wrapper.className = "firehose-block-wrapper live-incoming";
    wrapper.setAttribute("data-block-id", id);
    wrapper.setAttribute("data-block-source", source);
    wrapper.setAttribute("data-created-at", rawCreated);

    // Prefer server-provided rendered HTML when available; fallback to escaped plain text.
    const contentHTML = b.formatted_html
      ? b.formatted_html
      : escapeHtml(b.text || "");
    wrapper.innerHTML = `
      <div class="firehose-block-header">
        <span class="datasource-name"><a href="/datasource/${escapeHtml(
          source,
        )}">${escapeHtml(source)}</a></span>
        <span class="block-timestamp">${escapeHtml(createdDisplay)}</span>
      </div>
      <div class="firehose-block-content">${contentHTML}</div>
    `;

    // Fade-in effect / remove class after paint
    requestAnimationFrame(() => {
      wrapper.classList.remove("live-incoming");
      wrapper.classList.add("live-block");
    });

    updateCursorIfNewer(rawCreated);
    return wrapper;
  }

  function prependBlockElement(el) {
    if (!el || !blocksRoot) return;
    blocksRoot.prepend(el);
    trimDomIfNeeded();
  }

  function addBlock(b) {
    const el = createBlockElement(b);
    if (!el) return;
    prependBlockElement(el);
    // Increment insert counter & periodic pruning
    insertCount++;
    if (insertCount % PRUNE_INSERT_INTERVAL === 0) {
      // Fast prune
      trimDomIfNeeded();
    }
  }

  function processBlocks(arr) {
    if (!Array.isArray(arr)) return;
    for (const b of arr) {
      addBlock(b);
    }
  }

  // --- Initial Scan ---------------------------------------------------------

  function scanInitial() {
    const existing = blocksRoot.querySelectorAll(
      ".firehose-block-wrapper[data-block-id]",
    );
    existing.forEach((el) => {
      const id = el.getAttribute("data-block-id");
      const source = el.getAttribute("data-block-source") || "unknown";
      const created = el.getAttribute("data-created-at");
      if (id && source) seen.add(blockKey(source, id));
      const d = parseRFC3339(created);
      if (d && (!cursor || d > cursor)) cursor = d;
    });
    // If too many pre-rendered, prune to maxBlocks newest (should already be newest-first SSR).
    if (existing.length > cfg.maxBlocks) {
      for (let i = cfg.maxBlocks; i < existing.length; i++) {
        const node = existing[existing.length - 1 - (i - cfg.maxBlocks)];
        if (node && node.parentNode === blocksRoot) {
          blocksRoot.removeChild(node);
        }
      }
    }
  }

  // --- WebSocket Handling ---------------------------------------------------

  function buildWsUrl() {
    let qs = "";
    if (cursor) {
      // Use cursor with full precision; server internally normalizes per its rules.
      qs = "?since=" + encodeURIComponent(cursor.toISOString());
    }
    const base = cfg.wsPath.startsWith("ws")
      ? cfg.wsPath
      : (location.protocol === "https:" ? "wss://" : "ws://") +
        location.host +
        cfg.wsPath;
    return base + qs;
  }

  function scheduleReconnect() {
    if (reconnectTimer) return;
    setStatus("reconnecting", "Reconnecting…");
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      connect();
    }, backoff);
    backoff = Math.min(backoff * 2, cfg.reconnectMaxMs);
  }

  function handleMessage(msg) {
    switch (msg.type) {
      case "init":
        processBlocks(msg.blocks);
        if (msg.since) updateCursorIfNewer(msg.since);
        if (msg.mode) {
          connectionMode = msg.mode;
          console.debug(
            "[firehose] init mode=%s blocks=%d since=%s",
            connectionMode,
            msg.count,
            msg.since || "(none)",
          );
        } else {
          console.debug("[firehose] init (no mode field) blocks=%d", msg.count);
        }
        setStatus("live", "LIVE");
        break;
      case "block":
        addBlock(msg.block);
        setStatus("live", "Live");
        if (statusEl) {
          statusEl.classList.add("activity");
          setTimeout(
            () => statusEl && statusEl.classList.remove("activity"),
            500,
          );
        }
        break;
      case "block_batch":
        processBlocks(msg.blocks);
        if (msg.since) updateCursorIfNewer(msg.since);
        setStatus("live", "Live");
        if (statusEl) {
          statusEl.classList.add("activity");
          setTimeout(
            () => statusEl && statusEl.classList.remove("activity"),
            500,
          );
        }
        break;
      case "heartbeat":
        setStatus("live", "LIVE");
        break;
      case "error":
        setStatus("error", "Error");
        break;
      default:
        // ignore unknown types
        break;
    }
  }

  function connect() {
    if (ws && ws.readyState === WebSocket.OPEN) return;

    setStatus("connecting", "Connecting…");
    const url = buildWsUrl();
    console.debug(
      "[firehose] attempting websocket %s (cursor=%s)",
      url,
      cursor ? cursor.toISOString() : "none",
    );

    let connectTimeout = null;
    try {
      ws = new WebSocket(url);
    } catch (e) {
      console.warn("[firehose] WebSocket constructor failed:", e);
      scheduleReconnect();
      return;
    }

    // Failsafe: if still CONNECTING after 10s, force close to trigger backoff
    connectTimeout = setTimeout(() => {
      if (ws && ws.readyState === WebSocket.CONNECTING) {
        console.warn("[firehose] connect timeout, forcing close");
        try {
          ws.close();
        } catch (_) {}
      }
    }, 10000);

    ws.onopen = () => {
      clearTimeout(connectTimeout);
      backoff = cfg.reconnectBaseMs;
      console.debug("[firehose] websocket open");
      setStatus("live", "LIVE");
    };

    ws.onclose = (ev) => {
      clearTimeout(connectTimeout);
      console.debug(
        "[firehose] websocket closed code=%s reason=%s",
        ev.code,
        ev.reason,
      );
      ws = null;
      scheduleReconnect();
    };

    ws.onerror = (ev) => {
      console.warn("[firehose] websocket error", ev);
      setStatus("reconnecting", "Reconnecting…");
      if (ws) {
        try {
          ws.close();
        } catch (_) {}
      }
    };

    ws.onmessage = (ev) => {
      let msg;
      try {
        msg = JSON.parse(ev.data);
      } catch (e) {
        console.warn("[firehose] received non-JSON frame", e);
        return;
      }
      // Light verbose filtering to avoid spam
      if (msg.type !== "heartbeat") {
        console.debug("[firehose] message type=%s", msg.type);
      }
      handleMessage(msg);
    };
  }

  // --- Initialization -------------------------------------------------------
  //
  // Delay initial scan/connect if we are still waiting for the container.
  // The retry loop above will call scanInitial/connect when ready.

  if (!pendingInit) {
    scanInitial();
    connect();
  }
  // Periodic safety prune (full scan) to guard against any missed cases
  setInterval(() => trimDomIfNeeded(true), SAFETY_PRUNE_INTERVAL_MS);

  // Provide a simple API for debugging / manual control
  window.ergsFirehose = {
    reconnect: () => {
      if (ws) {
        try {
          ws.close();
        } catch (_) {
          /* ignore */
        }
      } else {
        connect();
      }
    },
    getState: () => ({
      cursor,
      seenCount: seen.size,
      socketReady: ws ? ws.readyState : "none",
      maxBlocks: cfg.maxBlocks,
    }),
  };
})();

/* Optional minimal styles (only if not handled by site CSS) */
(() => {
  if (document.getElementById("ergs-firehose-live-style")) return;
  const style = document.createElement("style");
  style.id = "ergs-firehose-live-style";
  style.textContent = `
    #firehose-live-status {
      display: inline-block;
      width: 7px;
      height: 7px;
      margin-left: .5rem;
      border-radius: 50%;
      position: relative;
      background: #fff;
      box-shadow: 0 0 4px 0 rgba(255,255,255,0.6);
      transition: background .25s ease, box-shadow .25s ease, transform .25s ease;
    }
    #firehose-live-status[data-state="initializing"],
    #firehose-live-status[data-state="connecting"],
    #firehose-live-status[data-state="reconnecting"] {
      background: #ff8c1a; /* orange */
      box-shadow: 0 0 6px 2px rgba(255,140,26,0.5);
    }
    #firehose-live-status[data-state="live"] {
      background: #fff;
      box-shadow: 0 0 4px 0 rgba(255,255,255,0.6);
    }
    #firehose-live-status[data-state="error"] {
      background: #c62828;
      box-shadow: 0 0 6px 2px rgba(198,40,40,0.6);
      animation: firehoseErrorPulse 1s ease-in-out infinite;
    }
    #firehose-live-status.activity {
      animation: firehoseFlash 0.5s ease;
    }
    @keyframes firehoseFlash {
      0% { background:#ff2727; box-shadow:0 0 6px 2px rgba(255,39,39,0.9); transform:scale(1.6); }
      100% { background:#fff; box-shadow:0 0 4px 0 rgba(255,255,255,0.6); transform:scale(1); }
    }
    @keyframes firehoseErrorPulse {
      0% { transform: scale(.85); }
      50% { transform: scale(1.05); }
      100% { transform: scale(.85); }
    }
    .firehose-block-wrapper.live-incoming {
      animation: ergsFadeIn .4s ease;
    }
    @keyframes ergsFadeIn {
      from { opacity: 0; transform: translateY(-4px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `;
  document.head.appendChild(style);
})();
