import type { PlatformGame } from "./api";

type Atlas = {
  playerIdle: HTMLImageElement;
  playerWalk1: HTMLImageElement;
  playerWalk2: HTMLImageElement;
  playerJump: HTMLImageElement;
  playerCrouch: HTMLImageElement;
  enemy: HTMLImageElement;
  bullet: HTMLImageElement;
  box: HTMLImageElement;
  boxHit: HTMLImageElement;
  item: HTMLImageElement;
  ground: HTMLImageElement;
  brick: HTMLImageElement;
  water: HTMLImageElement;
};

function loadImg(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error(`asset ${src}`));
    img.src = src;
  });
}

let atlas: Atlas | null = null;
let walkTick = 0;

export async function loadPlatformAssets(): Promise<void> {
  const base = "/assets/platform";
  const [
    playerIdle,
    playerWalk1,
    playerWalk2,
    playerJump,
    playerCrouch,
    enemy,
    bullet,
    box,
    boxHit,
    item,
    ground,
    brick,
    water,
  ] = await Promise.all([
    loadImg(`${base}/player_idle.png`),
    loadImg(`${base}/player_walk_1.png`),
    loadImg(`${base}/player_walk_2.png`),
    loadImg(`${base}/player_jump.png`),
    loadImg(`${base}/player_crouch.png`),
    loadImg(`${base}/enemy.png`),
    loadImg(`${base}/bullet.png`),
    loadImg(`${base}/box.png`),
    loadImg(`${base}/box_hit.png`),
    loadImg(`${base}/item_key.png`),
    loadImg(`${base}/tile_ground.png`),
    loadImg(`${base}/tile_brick.png`),
    loadImg(`${base}/tile_water.png`),
  ]);
  atlas = {
    playerIdle,
    playerWalk1,
    playerWalk2,
    playerJump,
    playerCrouch,
    enemy,
    bullet,
    box,
    boxHit,
    item,
    ground,
    brick,
    water,
  };
}

function worldToScreen(g: PlatformGame, x: number, y: number, h: number): { sx: number; sy: number } {
  const tile = g.tile_px;
  const sx = (x - g.camera_x) * tile;
  // y up in world → canvas y down
  const sy = (g.view_h - (y - 0) - h) * tile;
  return { sx, sy };
}

export function drawPlatform(canvas: HTMLCanvasElement, g: PlatformGame | null): void {
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const w = canvas.width;
  const h = canvas.height;
  ctx.fillStyle = "#87b7e0";
  ctx.fillRect(0, 0, w, h);

  if (!g) {
    ctx.fillStyle = "#1a2332";
    ctx.font = "16px Sora, sans-serif";
    ctx.fillText("横卷游戏判定 — 按开始", 24, 40);
    return;
  }

  const tile = g.tile_px;
  const a = atlas;

  // Pits: water tiles at the ground strip (same Y as tile_ground).
  // tile_water.png has a solid black top band meant as empty air — crop it.
  for (const pit of g.pits ?? []) {
    const a0 = pit[0];
    const a1 = pit[1];
    const sy = (g.view_h - g.ground_y) * tile;
    const pitH = Math.max(1, g.ground_y) * tile;
    for (let x = Math.floor(a0); x < Math.ceil(a1); x++) {
      const sx = (x - g.camera_x) * tile;
      if (a?.water) {
        const img = a.water;
        const srcW = img.naturalWidth || img.width;
        const srcH = img.naturalHeight || img.height;
        const skip = Math.floor(srcH * 0.28);
        ctx.drawImage(img, 0, skip, srcW, srcH - skip, sx, sy, tile, pitH);
      } else {
        ctx.fillStyle = "#2a6db0";
        ctx.fillRect(sx, sy, tile, pitH);
        ctx.fillStyle = "#7ec8f0";
        ctx.fillRect(sx, sy, tile, 4);
      }
    }
  }

  // ground surface + grey bricks filling the columns underneath
  for (const col of g.solids ?? []) {
    const sx = (col - g.camera_x) * tile;
    const syTop = (g.view_h - g.ground_y) * tile;
    if (a) ctx.drawImage(a.ground, sx, syTop, tile, tile);
    else {
      ctx.fillStyle = "#5a8f3c";
      ctx.fillRect(sx, syTop, tile, tile);
    }
    for (let row = 1; row < g.ground_y; row++) {
      const sy = syTop + row * tile;
      if (a?.brick) ctx.drawImage(a.brick, sx, sy, tile, tile);
      else {
        ctx.fillStyle = "#7a7e86";
        ctx.fillRect(sx, sy, tile, tile);
        ctx.strokeStyle = "#555860";
        ctx.strokeRect(sx + 0.5, sy + 0.5, tile - 1, tile - 1);
      }
    }
  }

  // boxes
  for (const b of g.boxes ?? []) {
    const { sx, sy } = worldToScreen(g, b.x - 0.5, b.y, 1);
    if (a) ctx.drawImage(b.hit ? a.boxHit : a.box, sx, sy, tile, tile);
    else {
      ctx.fillStyle = b.hit ? "#886644" : "#d4a24c";
      ctx.fillRect(sx, sy, tile, tile);
    }
  }

  // ground items (keys)
  for (const it of g.items ?? []) {
    if (!it.alive) continue;
    const { sx, sy } = worldToScreen(g, it.x - 0.5, it.y, 1);
    if (a) ctx.drawImage(a.item, sx, sy, tile, tile);
    else {
      ctx.fillStyle = "#f0be32";
      ctx.fillRect(sx + 8, sy + 8, tile - 16, tile - 16);
    }
  }

  // enemies
  for (const e of g.enemies ?? []) {
    if (!e.alive) continue;
    const { sx, sy } = worldToScreen(g, e.x - 0.5, e.y, 1);
    const faceLeft = (e.facing ?? -1) < 0;
    if (a) {
      ctx.save();
      if (faceLeft) {
        ctx.translate(sx + tile, sy);
        ctx.scale(-1, 1);
        ctx.drawImage(a.enemy, 0, 0, tile, tile);
      } else {
        ctx.drawImage(a.enemy, sx, sy, tile, tile);
      }
      ctx.restore();
    } else {
      ctx.fillStyle = "#c84a4a";
      ctx.fillRect(sx, sy, tile, tile);
    }
  }

  // bullets — visible core centered on physics hitbox
  for (const b of g.bullets ?? []) {
    const bw = Math.max(20, tile * 0.55);
    const bh = Math.max(12, tile * 0.36);
    const { sx, sy } = worldToScreen(g, b.x - bw / tile / 2, b.y, bh / tile);
    ctx.save();
    ctx.fillStyle = "rgba(255, 120, 40, 0.55)";
    ctx.beginPath();
    ctx.ellipse(sx + bw / 2, sy + bh / 2, bw * 0.55, bh * 0.55, 0, 0, Math.PI * 2);
    ctx.fill();
    if (a) ctx.drawImage(a.bullet, sx, sy, bw, bh);
    else {
      ctx.fillStyle = "#ffb020";
      ctx.fillRect(sx, sy, bw, bh);
    }
    ctx.strokeStyle = "#fff";
    ctx.lineWidth = 1.5;
    ctx.strokeRect(sx, sy, bw, bh);
    ctx.restore();
  }

  // player
  walkTick++;
  const ph = g.player_h;
  const { sx, sy } = worldToScreen(g, g.player_x - g.player_w / 2, g.player_y, ph);
  const pw = g.player_w * tile;
  const pph = ph * tile;
  const faceLeft = (g.facing ?? 1) < 0;
  let sprite = a?.playerIdle;
  if (a) {
    if (g.crouch) sprite = a.playerCrouch;
    else if (!g.grounded) sprite = a.playerJump;
    else if (g.move === "LEFT" || g.move === "RIGHT") {
      sprite = walkTick % 20 < 10 ? a.playerWalk1 : a.playerWalk2;
    } else sprite = a.playerIdle;
    const dw = 32;
    const dh = 64;
    const dx = sx - (dw - pw) / 2;
    const dy = sy - (dh - pph);
    ctx.save();
    if (faceLeft) {
      ctx.translate(dx + dw, dy);
      ctx.scale(-1, 1);
      ctx.drawImage(sprite!, 0, 0, dw, dh);
    } else {
      ctx.drawImage(sprite!, dx, dy, dw, dh);
    }
    ctx.restore();
  } else {
    ctx.fillStyle = "#3a84dc";
    ctx.fillRect(sx, sy, pw, pph);
  }

  if (!g.alive) {
    ctx.fillStyle = "rgba(0,0,0,0.45)";
    ctx.fillRect(0, 0, w, h);
    ctx.fillStyle = "#fff";
    ctx.font = "700 22px Sora, sans-serif";
    ctx.fillText(`Game Over — ${g.death_reason ?? "dead"}`, 24, 48);
    ctx.font = "14px IBM Plex Mono, monospace";
    ctx.fillText(`score ${g.score} · distance ${g.distance}`, 24, 76);
  }
}
