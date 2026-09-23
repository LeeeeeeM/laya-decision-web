import "./style.css";
import {
  controlPlatformSession,
  controlSession,
  createPlatformSession,
  createSession,
  deletePlatformSession,
  deleteSession,
  getCapabilities,
  getHealth,
  type DecisionEvent,
  type GameSnapshot,
  type PlatformGame,
  type PlatformSnapshot,
  type SessionSnapshot,
} from "./api";
import { drawBoard } from "./board";
import { drawPlatform, loadPlatformAssets } from "./platformBoard";

type GameKind = "snake" | "platform";

const els = {
  health: document.querySelector<HTMLDivElement>("#health")!,
  provider: document.querySelector<HTMLSelectElement>("#provider")!,
  mode: document.querySelector<HTMLSelectElement>("#mode")!,
  modeLabel: document.querySelector<HTMLLabelElement>("#mode-label")!,
  fps: document.querySelector<HTMLInputElement>("#fps")!,
  fpsLabel: document.querySelector<HTMLLabelElement>("#fps-label")!,
  seed: document.querySelector<HTMLInputElement>("#seed")!,
  start: document.querySelector<HTMLButtonElement>("#start")!,
  pause: document.querySelector<HTMLButtonElement>("#pause")!,
  resume: document.querySelector<HTMLButtonElement>("#resume")!,
  reset: document.querySelector<HTMLButtonElement>("#reset")!,
  stop: document.querySelector<HTMLButtonElement>("#stop")!,
  board: document.querySelector<HTMLCanvasElement>("#board")!,
  probs: document.querySelector<HTMLDivElement>("#probs")!,
  probsTitle: document.querySelector<HTMLHeadingElement>("#probs-title")!,
  log: document.querySelector<HTMLPreElement>("#log")!,
  session: document.querySelector("#stat-session")!,
  status: document.querySelector("#stat-status")!,
  score: document.querySelector("#stat-score")!,
  ticks: document.querySelector("#stat-ticks")!,
  proposed: document.querySelector("#stat-proposed")!,
  executed: document.querySelector("#stat-executed")!,
  intervened: document.querySelector("#stat-intervened")!,
  latency: document.querySelector("#stat-latency")!,
  hint: document.querySelector<HTMLParagraphElement>("#platform-hint")!,
  tabSnake: document.querySelector<HTMLButtonElement>("#tab-snake")!,
  tabPlatform: document.querySelector<HTMLButtonElement>("#tab-platform")!,
};

let gameKind: GameKind = "platform";
let sessionId: string | null = null;
let es: EventSource | null = null;
let snakeGame: GameSnapshot | null = null;
let platformGame: PlatformGame | null = null;
let decision: DecisionEvent | null = null;
let inputTimer: number | null = null;

const keys = { left: false, right: false, jump: false, crouch: false };

/** Match server BOCHA_JEV_MAX_FPS default (config.BochaJevMaxFPS). */
const BOCHA_JEV_MAX_RATE = 2;
const DEFAULT_RATE = 8;

function syncRateForProvider(): void {
  const provider = els.provider.value;
  const cur = Number(els.fps.value);
  if (provider === "bocha-jev") {
    els.fps.max = String(BOCHA_JEV_MAX_RATE);
    if (!Number.isFinite(cur) || cur > BOCHA_JEV_MAX_RATE) {
      els.fps.value = String(BOCHA_JEV_MAX_RATE);
    }
  } else {
    els.fps.max = "30";
    if (!Number.isFinite(cur) || cur <= 0) {
      els.fps.value = String(DEFAULT_RATE);
    }
  }
}

function rateValue(): number {
  syncRateForProvider();
  const n = Number(els.fps.value);
  return Number.isFinite(n) && n > 0 ? n : DEFAULT_RATE;
}

function log(msg: string): void {
  const line = `[${new Date().toLocaleTimeString()}] ${msg}`;
  els.log.textContent = `${line}\n${els.log.textContent ?? ""}`.slice(0, 4000);
}

/** Empty / invalid seed → 0 so the server picks a random map. */
function seedValue(): number {
  const raw = els.seed.value.trim();
  if (!raw) return 0;
  const n = Number(raw);
  return Number.isFinite(n) ? n : 0;
}

function setBusy(running: boolean): void {
  els.start.disabled = running;
  els.pause.disabled = !running;
  els.resume.disabled = !running;
  els.reset.disabled = !running;
  els.stop.disabled = !running;
  els.provider.disabled = running;
  els.mode.disabled = running;
  els.tabSnake.disabled = running;
  els.tabPlatform.disabled = running;
}

function renderProbs(): void {
  if (gameKind === "snake") {
    els.probsTitle.textContent = "方向概率";
    const dirs = ["UP", "DOWN", "LEFT", "RIGHT"];
    const probs = decision?.probabilities ?? null;
    els.probs.innerHTML = dirs
      .map((d) => {
        const v = probs?.[d] ?? 0;
        const pct = Math.round(v * 1000) / 10;
        return `<div class="prob-row"><span>${d}</span><div class="bar"><span style="width:${pct}%"></span></div><span>${pct}%</span></div>`;
      })
      .join("");
    return;
  }
  // Bars show raw model probabilities; strategy cues can override the executed action.
  els.probsTitle.textContent = "动作概率（模型）";
  const move = decision?.prob_move ?? {};
  const action = decision?.prob_action ?? {};
  const execMove = (decision?.move ?? decision?.executed?.split("+")[0] ?? "").toUpperCase();
  const execAction = (
    decision?.action ?? decision?.executed?.split("+")[1] ?? ""
  ).toUpperCase();
  const rows = [
    ...["LEFT", "RIGHT", "IDLE"].map(
      (k) => ["move", k, move[k] ?? 0, k === execMove] as const,
    ),
    ...["NONE", "JUMP", "CROUCH"].map(
      (k) => ["action", k, action[k] ?? 0, k === execAction] as const,
    ),
  ];
  els.probs.innerHTML = rows
    .map(([axis, k, v, exec]) => {
      const pct = Math.round(Number(v) * 1000) / 10;
      const cls = exec ? "prob-row exec" : "prob-row";
      const mark = exec ? " ←实际" : "";
      return `<div class="${cls}"><span>${axis} ${k}${mark}</span><div class="bar"><span style="width:${pct}%"></span></div><span>${pct}%</span></div>`;
    })
    .join("");
}

function redraw(): void {
  if (gameKind === "snake") drawBoard(els.board, snakeGame, decision);
  else drawPlatform(els.board, platformGame);
  renderProbs();
}

function updateSnakeStats(snap?: SessionSnapshot): void {
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
  }
  redraw();
}

function updatePlatformStats(snap?: PlatformSnapshot): void {
  if (snap) {
    els.session.textContent = snap.session_id;
    els.status.textContent = snap.status;
    els.score.textContent = String(snap.game.score);
    els.ticks.textContent = String(snap.game.distance);
    platformGame = snap.game;
  }
  if (decision) {
    els.proposed.textContent = decision.proposed;
    els.executed.textContent = decision.executed;
    els.intervened.textContent = decision.intervened ? "yes" : "no";
    els.latency.textContent = `${decision.inference_ms.toFixed(1)} ms`;
  }
  redraw();
}

function selectGame(kind: GameKind): void {
  gameKind = kind;
  els.tabSnake.classList.toggle("active", kind === "snake");
  els.tabPlatform.classList.toggle("active", kind === "platform");
  els.modeLabel.classList.toggle("hidden", kind !== "platform");
  els.hint.classList.toggle("hidden", kind !== "platform");
  updatePlatformHint();
  const fpsName = document.querySelector("#fps-name");
  if (fpsName) fpsName.textContent = kind === "platform" ? "Decision Hz" : "FPS";
  if (kind === "platform") {
    els.board.width = 768;
    els.board.height = 448;
  } else {
    els.board.width = 720;
    els.board.height = 480;
  }
  redraw();
}

function updatePlatformHint(): void {
  if (gameKind !== "platform") return;
  const mode = els.mode.value === "human" ? "人控" : "Agent";
  els.hint.textContent =
    `← → 移动 · Z/空格 跳跃 · ↓/C 蹲下（躲弹 / 捡钥匙）· 当前：${mode}`;
}

async function stopSession(): Promise<void> {
  if (inputTimer != null) {
    window.clearInterval(inputTimer);
    inputTimer = null;
  }
  if (es) {
    es.close();
    es = null;
  }
  if (sessionId) {
    try {
      if (gameKind === "snake") await deleteSession(sessionId);
      else await deletePlatformSession(sessionId);
    } catch {
      /* ignore */
    }
    sessionId = null;
  }
  setBusy(false);
}

function bindStream(url: string): void {
  es = new EventSource(url);
  es.addEventListener("snapshot", (ev) => {
    const data = JSON.parse((ev as MessageEvent).data);
    if (gameKind === "snake") {
      const snap = data as SessionSnapshot;
      snakeGame = snap.game;
      updateSnakeStats(snap);
    } else {
      // Platform SSE emits SessionSnapshot { game: PlatformGame }
      const snap = data as PlatformSnapshot;
      if (snap.game?.player_x != null) {
        updatePlatformStats(snap);
      } else if ((data as PlatformGame).player_x != null) {
        platformGame = data as PlatformGame;
        redraw();
      }
    }
  });
  es.addEventListener("decision", (ev) => {
    decision = JSON.parse((ev as MessageEvent).data) as DecisionEvent;
    if (gameKind === "snake") updateSnakeStats();
    else updatePlatformStats();
  });
  es.addEventListener("status", (ev) => {
    const data = JSON.parse((ev as MessageEvent).data);
    els.status.textContent = data.status ?? els.status.textContent;
    if (data.status === "finished" || data.status === "stopped") {
      log(`status ${data.status}`);
    }
  });
  es.onerror = () => log("SSE disconnected");
}

function startInputLoop(): void {
  if (inputTimer != null) return;
  inputTimer = window.setInterval(() => {
    if (!sessionId || gameKind !== "platform" || els.mode.value !== "human") return;
    void controlPlatformSession(sessionId, {
      action: "input",
      keys: { ...keys },
    }).catch(() => undefined);
  }, 40);
}

async function boot(): Promise<void> {
  try {
    await loadPlatformAssets();
  } catch (err) {
    log(`assets: ${err}`);
  }
  selectGame("platform");
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
      opt.textContent = p.available ? p.id : `${p.id} (unavailable)`;
      opt.disabled = !p.available;
      els.provider.appendChild(opt);
      if (p.id === "laya" && p.available) els.provider.value = "laya";
    }
    syncRateForProvider();
  } catch (err) {
    log(String(err));
  }

  els.provider.addEventListener("change", () => syncRateForProvider());

  els.tabSnake.addEventListener("click", () => {
    if (!sessionId) selectGame("snake");
  });
  els.tabPlatform.addEventListener("click", () => {
    if (!sessionId) selectGame("platform");
  });
  els.mode.addEventListener("change", () => updatePlatformHint());

  els.start.addEventListener("click", async () => {
    await stopSession();
    decision = null;
    try {
      const rate = rateValue();
      if (gameKind === "snake") {
        const created = await createSession({
          provider: els.provider.value,
          fps: rate,
          seed: seedValue(),
          prompt: "compact",
        });
        sessionId = created.session_id;
        snakeGame = created.snapshot;
        setBusy(true);
        bindStream(created.events_url);
        updateSnakeStats({
          session_id: created.session_id,
          status: created.status,
          provider: els.provider.value,
          model: "",
          fps: rate,
          prompt: "compact",
          game: created.snapshot,
        });
        log(`snake session ${sessionId}`);
      } else {
        const created = await createPlatformSession({
          provider: els.provider.value,
          seed: seedValue(),
          decision_hz: rate,
          mode: els.mode.value,
        });
        sessionId = created.session_id;
        setBusy(true);
        bindStream(created.events_url);
        updatePlatformStats(created.snapshot);
        if (els.mode.value === "human") startInputLoop();
        log(`platform session ${sessionId} mode=${els.mode.value}`);
      }
    } catch (err) {
      log(String(err));
      setBusy(false);
    }
  });

  const ctrl = async (action: string) => {
    if (!sessionId) return;
    try {
      if (gameKind === "snake") {
        const res = await controlSession(sessionId, { action, seed: seedValue() });
        updateSnakeStats(res.snapshot);
      } else {
        const res = await controlPlatformSession(sessionId, {
          action,
          seed: seedValue() || undefined,
        });
        updatePlatformStats(res.snapshot);
      }
      log(`control ${action}`);
    } catch (err) {
      log(String(err));
    }
  };

  els.pause.addEventListener("click", () => void ctrl("pause"));
  els.resume.addEventListener("click", () => void ctrl("resume"));
  els.reset.addEventListener("click", () => void ctrl("reset"));
  els.stop.addEventListener("click", () => void stopSession().then(() => log("stopped")));

  window.addEventListener("keydown", (e) => {
    if (gameKind !== "platform") return;
    if (e.code === "ArrowLeft" || e.code === "KeyA") keys.left = true;
    if (e.code === "ArrowRight" || e.code === "KeyD") keys.right = true;
    if (e.code === "KeyZ" || e.code === "Space" || e.code === "KeyX") keys.jump = true;
    if (e.code === "ArrowDown" || e.code === "KeyC" || e.code === "KeyS") keys.crouch = true;
  });
  window.addEventListener("keyup", (e) => {
    if (e.code === "ArrowLeft" || e.code === "KeyA") keys.left = false;
    if (e.code === "ArrowRight" || e.code === "KeyD") keys.right = false;
    if (e.code === "KeyZ" || e.code === "Space" || e.code === "KeyX") keys.jump = false;
    if (e.code === "ArrowDown" || e.code === "KeyC" || e.code === "KeyS") keys.crouch = false;
  });
}

void boot();
