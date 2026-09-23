import type { DecisionEvent, GameSnapshot } from "./api";
import { drawScoreHUD } from "./canvasHud";

const DIRS = ["UP", "DOWN", "LEFT", "RIGHT"] as const;

export function drawBoard(
  canvas: HTMLCanvasElement,
  game: GameSnapshot | null,
  decision: DecisionEvent | null,
): void {
  const ctx = canvas.getContext("2d");
  if (!ctx) return;

  const w = game?.width ?? 24;
  const h = game?.height ?? 16;
  const cell = Math.min(canvas.width / w, canvas.height / h);
  const ox = (canvas.width - cell * w) / 2;
  const oy = (canvas.height - cell * h) / 2;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#07110d";
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  ctx.strokeStyle = "rgba(232,240,234,0.06)";
  ctx.lineWidth = 1;
  for (let x = 0; x <= w; x++) {
    ctx.beginPath();
    ctx.moveTo(ox + x * cell, oy);
    ctx.lineTo(ox + x * cell, oy + h * cell);
    ctx.stroke();
  }
  for (let y = 0; y <= h; y++) {
    ctx.beginPath();
    ctx.moveTo(ox, oy + y * cell);
    ctx.lineTo(ox + w * cell, oy + y * cell);
    ctx.stroke();
  }

  if (!game) {
    drawScoreHUD(ctx, canvas.width, 0);
    return;
  }

  if (game.food) {
    const [fx, fy] = game.food;
    ctx.fillStyle = "#e0a45a";
    roundRect(ctx, ox + fx * cell + 2, oy + fy * cell + 2, cell - 4, cell - 4, 4);
    ctx.fill();
  }

  game.body.forEach((seg, i) => {
    const [x, y] = seg;
    const t = i === 0 ? 1 : Math.max(0.35, 1 - i / Math.max(game.body.length, 1));
    ctx.fillStyle = `rgba(61, 186, 126, ${t})`;
    roundRect(ctx, ox + x * cell + 1.5, oy + y * cell + 1.5, cell - 3, cell - 3, 3);
    ctx.fill();
  });

  if (decision?.executed && game.body[0]) {
    const [hx, hy] = game.body[0];
    ctx.strokeStyle = decision.intervened ? "#e0a45a" : "#7ddea8";
    ctx.lineWidth = 2;
    roundRect(ctx, ox + hx * cell + 1, oy + hy * cell + 1, cell - 2, cell - 2, 4);
    ctx.stroke();
  }

  drawScoreHUD(ctx, canvas.width, game.score);
  void DIRS;
}

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
): void {
  const radius = Math.min(r, w / 2, h / 2);
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.arcTo(x + w, y, x + w, y + h, radius);
  ctx.arcTo(x + w, y + h, x, y + h, radius);
  ctx.arcTo(x, y + h, x, y, radius);
  ctx.arcTo(x, y, x + w, y, radius);
  ctx.closePath();
}
