import "./style.css";
import {
  controlSession,
  createSession,
  deleteSession,
  getCapabilities,
  getHealth,
  type DecisionEvent,
  type GameSnapshot,
  type SessionSnapshot,
} from "./api";
import { drawBoard } from "./board";

const els = {
  health: document.querySelector<HTMLDivElement>("#health")!,
  provider: document.querySelector<HTMLSelectElement>("#provider")!,
  fps: document.querySelector<HTMLInputElement>("#fps")!,
  seed: document.querySelector<HTMLInputElement>("#seed")!,
  guarded: document.querySelector<HTMLInputElement>("#guarded")!,
  start: document.querySelector<HTMLButtonElement>("#start")!,
  pause: document.querySelector<HTMLButtonElement>("#pause")!,
  resume: document.querySelector<HTMLButtonElement>("#resume")!,
  reset: document.querySelector<HTMLButtonElement>("#reset")!,
  stop: document.querySelector<HTMLButtonElement>("#stop")!,
  board: document.querySelector<HTMLCanvasElement>("#board")!,
  probs: document.querySelector<HTMLDivElement>("#probs")!,
  log: document.querySelector<HTMLPreElement>("#log")!,
  session: document.querySelector("#stat-session")!,
  status: document.querySelector("#stat-status")!,
  score: document.querySelector("#stat-score")!,
  ticks: document.querySelector("#stat-ticks")!,
  proposed: document.querySelector("#stat-proposed")!,
  executed: document.querySelector("#stat-executed")!,
  intervened: document.querySelector("#stat-intervened")!,
  latency: document.querySelector("#stat-latency")!,
};

let sessionId: string | null = null;
let es: EventSource | null = null;
let game: GameSnapshot | null = null;
let decision: DecisionEvent | null = null;

function log(msg: string): void {
  const line = `[${new Date().toLocaleTimeString()}] ${msg}`;
  els.log.textContent = `${line}\n${els.log.textContent ?? ""}`.slice(0, 4000);
}

function setBusy(running: boolean): void {
  els.start.disabled = running;
  els.pause.disabled = !running;
  els.resume.disabled = !running;
  els.reset.disabled = !running;
  els.stop.disabled = !running;
  els.provider.disabled = running;
}

function renderProbs(probs: Record<string, number> | null): void {
  const dirs = ["UP", "DOWN", "LEFT", "RIGHT"];
  els.probs.innerHTML = dirs
    .map((d) => {
      const v = probs?.[d] ?? 0;
      const pct = Math.round(v * 1000) / 10;
      return `<div class="prob-row"><span>${d}</span><div class="bar"><span style="width:${pct}%"></span></div><span>${pct}%</span></div>`;
    })
    .join("");
}

function updateStats(snap?: SessionSnapshot): void {
  if (snap) {
    els.session.textContent = snap.session_id;
    els.status.textContent = snap.status;
    els.score.textContent = String(snap.game.score);
    els.ticks.textContent = String(snap.game.ticks);
  }
  if (decision) {
    els.proposed.textContent = decision.proposed;
    els.executed.textContent = decision.executed;
    els.intervened.textContent = decision.intervened ? "yes" : "no";
    els.latency.textContent = `${decision.inference_ms.toFixed(1)} ms`;
    renderProbs(decision.probabilities);
  }
  drawBoard(els.board, game, decision);
}

async function boot(): Promise<void> {
  renderProbs(null);
  drawBoard(els.board, null, null);
  try {
    const health = await getHealth();
    els.health.textContent = `ok · v${health.version}`;
  } catch (err) {
    els.health.textContent = "backend offline";
    log(String(err));
  }

  try {
    const caps = await getCapabilities();
    els.provider.innerHTML = "";
    for (const p of caps.providers) {
      const opt = document.createElement("option");
      opt.value = p.id;
      opt.disabled = !p.available;
      opt.textContent = p.available ? `${p.id}${p.model ? ` (${p.model})` : ""}` : `${p.id} (unavailable)`;
      els.provider.appendChild(opt);
    }
    const preferred = caps.providers.find((p) => p.id === "laya" && p.available)
      ?? caps.providers.find((p) => p.id === "mock" && p.available)
      ?? caps.providers.find((p) => p.available);
    if (preferred) els.provider.value = preferred.id;
    applyProviderDefaults(els.provider.value);
  } catch (err) {
    log(String(err));
  }
}

function applyProviderDefaults(provider: string): void {
  if (provider === "bocha-jev") els.fps.value = "1";
  else if (provider === "laya") els.fps.value = "12";
  else if (provider === "mock") els.fps.value = "8";
}

function connectEvents(id: string): void {
  es?.close();
  es = new EventSource(`/api/v1/sessions/${id}/events`);
  es.addEventListener("snapshot", (ev) => {
    const snap = JSON.parse((ev as MessageEvent).data) as SessionSnapshot;
    game = snap.game;
    updateStats(snap);
    if (snap.status === "finished" || snap.status === "stopped") {
      setBusy(false);
    }
  });
  es.addEventListener("decision", (ev) => {
    decision = JSON.parse((ev as MessageEvent).data) as DecisionEvent;
    updateStats();
  });
  es.addEventListener("status", (ev) => {
    const data = JSON.parse((ev as MessageEvent).data) as { status: string };
    els.status.textContent = data.status;
    log(`status → ${data.status}`);
  });
  es.addEventListener("error", (ev) => {
    if ((ev as MessageEvent).data) {
      try {
        const data = JSON.parse((ev as MessageEvent).data) as { message?: string };
        log(data.message ?? "error");
      } catch {
        log("sse error");
      }
    }
  });
  es.onerror = () => {
    log("event stream disconnected");
  };
}

els.provider.addEventListener("change", () => {
  applyProviderDefaults(els.provider.value);
});

els.start.addEventListener("click", async () => {
  try {
    if (sessionId) {
      await deleteSession(sessionId);
      sessionId = null;
    }
    decision = null;
    const created = await createSession({
      provider: els.provider.value,
      fps: Number(els.fps.value),
      seed: Number(els.seed.value),
      guarded: els.guarded.checked,
      prompt: "compact",
    });
    sessionId = created.session_id;
    game = created.snapshot;
    setBusy(true);
    connectEvents(sessionId);
    log(`session ${sessionId} started`);
    updateStats({
      session_id: sessionId,
      status: created.status,
      provider: els.provider.value,
      model: "",
      fps: Number(els.fps.value),
      guarded: els.guarded.checked,
      prompt: "compact",
      game: created.snapshot,
    });
  } catch (err) {
    log(String(err));
  }
});

async function sendControl(action: string, extra: Record<string, number> = {}): Promise<void> {
  if (!sessionId) return;
  try {
    const res = await controlSession(sessionId, { action, ...extra });
    game = res.snapshot.game;
    updateStats(res.snapshot);
    log(`${action} ok`);
  } catch (err) {
    log(String(err));
  }
}

els.pause.addEventListener("click", () => void sendControl("pause"));
els.resume.addEventListener("click", () => void sendControl("resume"));
els.reset.addEventListener("click", () => void sendControl("reset", { seed: Number(els.seed.value) }));
els.stop.addEventListener("click", async () => {
  if (!sessionId) return;
  await sendControl("stop");
  es?.close();
  es = null;
  await deleteSession(sessionId);
  sessionId = null;
  setBusy(false);
});

els.fps.addEventListener("change", () => {
  if (sessionId) void sendControl("set_speed", { fps: Number(els.fps.value) });
});

void boot();
