export function drawScoreHUD(
  ctx: CanvasRenderingContext2D,
  canvasWidth: number,
  score: number,
): void {
  const scoreText = String(score);
  const cssWidth = ctx.canvas.clientWidth || canvasWidth;
  const scale = canvasWidth / cssWidth;
  const labelFontSize = 12 * scale;
  const scoreFontSize = 22 * scale;
  const paddingX = 12 * scale;
  ctx.save();

  ctx.font = `600 ${labelFontSize}px IBM Plex Mono, monospace`;
  const labelWidth = ctx.measureText("SCORE").width;
  ctx.font = `700 ${scoreFontSize}px Sora, sans-serif`;
  const scoreWidth = ctx.measureText(scoreText).width;

  const boxWidth = Math.max(82 * scale, Math.ceil(Math.max(labelWidth, scoreWidth)) + paddingX * 2);
  const boxHeight = 52 * scale;
  const margin = 14 * scale;
  const radius = 10 * scale;
  const x = canvasWidth - boxWidth - margin;
  const y = margin;

  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.arcTo(x + boxWidth, y, x + boxWidth, y + boxHeight, radius);
  ctx.arcTo(x + boxWidth, y + boxHeight, x, y + boxHeight, radius);
  ctx.arcTo(x, y + boxHeight, x, y, radius);
  ctx.arcTo(x, y, x + boxWidth, y, radius);
  ctx.closePath();
  ctx.fillStyle = "rgba(8, 18, 30, 0.84)";
  ctx.fill();
  ctx.strokeStyle = "rgba(255, 255, 255, 0.18)";
  ctx.lineWidth = 1;
  ctx.stroke();

  ctx.textAlign = "left";
  ctx.textBaseline = "alphabetic";
  ctx.font = `600 ${labelFontSize}px IBM Plex Mono, monospace`;
  ctx.fillStyle = "#c4d1df";
  ctx.fillText("SCORE", x + paddingX, y + 17 * scale);
  ctx.font = `700 ${scoreFontSize}px Sora, sans-serif`;
  ctx.fillStyle = "#f6d365";
  ctx.fillText(scoreText, x + paddingX, y + 42 * scale);
  ctx.restore();
}
