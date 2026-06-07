const fs = require('fs');
const path = require('path');
const { PNG } = require('pngjs');

const ASSET_DIR = path.resolve(__dirname, '..', 'assets');

const colors = {
  teal: [24, 181, 166, 255],
  blue: [58, 111, 232, 255],
  ink: [28, 34, 45, 255],
  panel: [255, 211, 91, 255],
  panelLight: [255, 229, 143, 255],
  coral: [255, 103, 96, 255],
  white: [248, 250, 252, 255],
  transparent: [0, 0, 0, 0],
};

function mix(a, b, t) {
  return [
    Math.round(a[0] + (b[0] - a[0]) * t),
    Math.round(a[1] + (b[1] - a[1]) * t),
    Math.round(a[2] + (b[2] - a[2]) * t),
    Math.round(a[3] + (b[3] - a[3]) * t),
  ];
}

function createPng(size, fill = colors.transparent) {
  const png = new PNG({ width: size, height: size, colorType: 6 });
  for (let y = 0; y < size; y += 1) {
    for (let x = 0; x < size; x += 1) {
      setPixel(png, x, y, fill);
    }
  }
  return png;
}

function setPixel(png, x, y, color) {
  if (x < 0 || y < 0 || x >= png.width || y >= png.height) return;
  const index = (Math.floor(y) * png.width + Math.floor(x)) * 4;
  const alpha = color[3] / 255;
  const inverse = 1 - alpha;
  png.data[index] = Math.round(color[0] * alpha + png.data[index] * inverse);
  png.data[index + 1] = Math.round(color[1] * alpha + png.data[index + 1] * inverse);
  png.data[index + 2] = Math.round(color[2] * alpha + png.data[index + 2] * inverse);
  png.data[index + 3] = Math.min(255, Math.round(color[3] + png.data[index + 3] * inverse));
}

function fillBackground(png) {
  const size = png.width;
  for (let y = 0; y < size; y += 1) {
    for (let x = 0; x < size; x += 1) {
      const nx = x / (size - 1);
      const ny = y / (size - 1);
      let color = mix(colors.teal, colors.blue, (nx + ny) / 2);
      const warm = Math.max(0, 1 - Math.hypot(nx - 0.22, ny - 0.18) / 0.74);
      color = mix(color, [255, 198, 73, 255], warm * 0.28);
      setPixel(png, x, y, color);
    }
  }
}

function roundRect(png, x, y, w, h, r, color) {
  const x2 = x + w;
  const y2 = y + h;
  for (let yy = Math.floor(y); yy < y2; yy += 1) {
    for (let xx = Math.floor(x); xx < x2; xx += 1) {
      const dx = Math.max(x - xx, 0, xx - x2 + 1);
      const dy = Math.max(y - yy, 0, yy - y2 + 1);
      const insideCore = (xx >= x + r && xx <= x2 - r) || (yy >= y + r && yy <= y2 - r);
      const cx = xx < x + r ? x + r : xx > x2 - r ? x2 - r : xx;
      const cy = yy < y + r ? y + r : yy > y2 - r ? y2 - r : yy;
      const insideCorner = Math.hypot(xx - cx, yy - cy) <= r;
      if (insideCore || insideCorner || (dx === 0 && dy === 0)) setPixel(png, xx, yy, color);
    }
  }
}

function circle(png, cx, cy, radius, color) {
  const minX = Math.floor(cx - radius);
  const maxX = Math.ceil(cx + radius);
  const minY = Math.floor(cy - radius);
  const maxY = Math.ceil(cy + radius);
  const r2 = radius * radius;
  for (let y = minY; y <= maxY; y += 1) {
    for (let x = minX; x <= maxX; x += 1) {
      if ((x - cx) ** 2 + (y - cy) ** 2 <= r2) setPixel(png, x, y, color);
    }
  }
}

function ring(png, cx, cy, radius, stroke, color) {
  const minX = Math.floor(cx - radius - stroke);
  const maxX = Math.ceil(cx + radius + stroke);
  const minY = Math.floor(cy - radius - stroke);
  const maxY = Math.ceil(cy + radius + stroke);
  const outer = (radius + stroke / 2) ** 2;
  const inner = (radius - stroke / 2) ** 2;
  for (let y = minY; y <= maxY; y += 1) {
    for (let x = minX; x <= maxX; x += 1) {
      const d2 = (x - cx) ** 2 + (y - cy) ** 2;
      if (d2 <= outer && d2 >= inner) setPixel(png, x, y, color);
    }
  }
}

function line(png, x1, y1, x2, y2, width, color) {
  const minX = Math.floor(Math.min(x1, x2) - width);
  const maxX = Math.ceil(Math.max(x1, x2) + width);
  const minY = Math.floor(Math.min(y1, y2) - width);
  const maxY = Math.ceil(Math.max(y1, y2) + width);
  const dx = x2 - x1;
  const dy = y2 - y1;
  const length2 = dx * dx + dy * dy;
  for (let y = minY; y <= maxY; y += 1) {
    for (let x = minX; x <= maxX; x += 1) {
      const t = Math.max(0, Math.min(1, ((x - x1) * dx + (y - y1) * dy) / length2));
      const px = x1 + t * dx;
      const py = y1 + t * dy;
      if (Math.hypot(x - px, y - py) <= width / 2) setPixel(png, x, y, color);
    }
  }
}

function arc(png, cx, cy, radius, start, end, width, color) {
  const steps = 180;
  let previous = null;
  for (let i = 0; i <= steps; i += 1) {
    const t = start + (end - start) * (i / steps);
    const x = cx + Math.cos(t) * radius;
    const y = cy + Math.sin(t) * radius;
    if (previous) line(png, previous.x, previous.y, x, y, width, color);
    previous = { x, y };
  }
}

function drawLogo(png, scale = 1, originX = 0, originY = 0, includeBackground = true) {
  const s = (value) => originX + value * scale;
  const sy = (value) => originY + value * scale;

  if (includeBackground) {
    fillBackground(png);
    circle(png, s(830), sy(160), 86 * scale, [255, 255, 255, 32]);
    circle(png, s(170), sy(840), 132 * scale, [255, 255, 255, 24]);
    line(png, s(90), sy(210), s(934), sy(818), 20 * scale, [255, 255, 255, 30]);
  }

  roundRect(png, s(232), sy(236), 542 * scale, 500 * scale, 128 * scale, [11, 25, 38, 54]);
  roundRect(png, s(214), sy(210), 542 * scale, 500 * scale, 128 * scale, colors.panel);
  roundRect(png, s(248), sy(250), 474 * scale, 178 * scale, 76 * scale, colors.panelLight);

  circle(png, s(380), sy(452), 44 * scale, colors.ink);
  circle(png, s(582), sy(452), 44 * scale, colors.ink);
  circle(png, s(396), sy(436), 13 * scale, colors.white);
  circle(png, s(598), sy(436), 13 * scale, colors.white);
  circle(png, s(316), sy(552), 30 * scale, [255, 130, 118, 160]);
  circle(png, s(650), sy(552), 30 * scale, [255, 130, 118, 160]);
  arc(png, s(484), sy(520), 118 * scale, 0.25, Math.PI - 0.25, 22 * scale, colors.ink);

  ring(png, s(548), sy(536), 220 * scale, 44 * scale, colors.white);
  ring(png, s(548), sy(536), 220 * scale, 15 * scale, [28, 34, 45, 72]);
  line(png, s(690), sy(694), s(850), sy(854), 70 * scale, colors.white);
  line(png, s(690), sy(694), s(850), sy(854), 30 * scale, colors.ink);

  circle(png, s(228), sy(226), 40 * scale, colors.coral);
  circle(png, s(792), sy(306), 28 * scale, [255, 255, 255, 180]);
  circle(png, s(818), sy(278), 15 * scale, colors.coral);
}

function downsample(source, targetSize) {
  const target = createPng(targetSize);
  const ratio = source.width / targetSize;
  const samples = Math.floor(ratio) ** 2;
  for (let y = 0; y < targetSize; y += 1) {
    for (let x = 0; x < targetSize; x += 1) {
      const total = [0, 0, 0, 0];
      for (let yy = 0; yy < ratio; yy += 1) {
        for (let xx = 0; xx < ratio; xx += 1) {
          const sx = Math.floor(x * ratio + xx);
          const sy = Math.floor(y * ratio + yy);
          const index = (sy * source.width + sx) * 4;
          total[0] += source.data[index];
          total[1] += source.data[index + 1];
          total[2] += source.data[index + 2];
          total[3] += source.data[index + 3];
        }
      }
      setPixel(target, x, y, total.map((value) => Math.round(value / samples)));
    }
  }
  return target;
}

function writePng(fileName, png) {
  fs.writeFileSync(path.join(ASSET_DIR, fileName), PNG.sync.write(png));
}

function makeIcon(size) {
  const scale = 4;
  const large = createPng(size * scale);
  drawLogo(large, scale, 0, 0, true);
  return downsample(large, size);
}

function makeSplash(size) {
  const scale = 4;
  const large = createPng(size * scale, colors.transparent);
  drawLogo(large, 2.5, 256 * scale, 190 * scale, false);
  return downsample(large, size);
}

writePng('icon.png', makeIcon(1024));
writePng('adaptive-icon.png', makeIcon(1024));
writePng('splash-icon.png', makeSplash(1024));
writePng('favicon.png', makeIcon(48));

console.log('Generated emomo mobile brand assets.');
