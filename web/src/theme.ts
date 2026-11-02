import { webApp, type ThemeParams } from "./telegram";

// WCAG 2.2 AA contrast for text (1.4.3). The input outline uses the hint color, so it passes
// the 3:1 for controls (1.4.11) too.
const textContrast = 4.5;

// Colors for anything Telegram leaves out, close to its default themes.
const defaults = {
  light: { page: "#f4f4f5", card: "#ffffff", text: "#000000", hint: "#707579", accent: "#3390ec", danger: "#e53935", ok: "#2e7d32", warn: "#b45309" },
  dark: { page: "#0f0f0f", card: "#212121", text: "#ffffff", hint: "#aaaaaa", accent: "#8774e1", danger: "#ff595a", ok: "#66bb6a", warn: "#ffa726" },
};

type RGB = [number, number, number];

// rgb parses #rgb or #rrggbb, the format Telegram sends.
function rgb(hex: string): RGB {
  const h = hex.replace("#", "");
  const full = h.length === 3 ? [...h].map((c) => c + c).join("") : h;
  return [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16)) as RGB;
}

function luminance(c: RGB): number {
  const [r, g, b] = c.map((v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  }) as RGB;
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

// contrast is the WCAG contrast ratio of two colors.
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(rgb(a)), luminance(rgb(b))].sort((x, y) => y - x) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
}

// readable returns color, moved toward black on light backgrounds or toward white on dark ones
// until it reaches AA on every one of bgs. A color that passes is returned unchanged.
function readable(color: string, bgs: string[]): string {
  const to = luminance(rgb(bgs[0] ?? "#fff")) > 0.5 ? 0 : 255;
  const from = rgb(color);
  let c = color;
  for (let t = 0.1; t <= 1 && bgs.some((bg) => contrast(c, bg) < textContrast); t += 0.1) {
    c = `#${from.map((v) => Math.round(v + (to - v) * t).toString(16).padStart(2, "0")).join("")}`;
  }
  return c;
}

// label returns text if it reads on bg, else black or white, whichever reads better.
function label(text: string | undefined, bg: string): string {
  if (text && contrast(text, bg) >= textContrast) return text;
  return contrast("#000000", bg) > contrast("#ffffff", bg) ? "#000000" : "#ffffff";
}

function colors(scheme: "light" | "dark", p: ThemeParams): Record<string, string> {
  const d = defaults[scheme];
  const page = p.secondary_bg_color ?? d.page;
  const card = p.section_bg_color ?? p.bg_color ?? d.card;
  const bgs = [card, page];
  const accent = readable(p.button_color ?? d.accent, bgs);
  const danger = readable(p.destructive_text_color ?? d.danger, bgs);
  return {
    page,
    card,
    text: readable(p.text_color ?? d.text, bgs),
    hint: readable(p.hint_color ?? d.hint, bgs),
    accent,
    "on-accent": label(p.button_text_color, accent),
    danger,
    "on-danger": label("#ffffff", danger),
    ok: readable(d.ok, bgs),
    warn: readable(d.warn, bgs),
  };
}

// applyTheme sets the page colors from the user's Telegram theme, in light or dark mode as
// Telegram is. Outside Telegram it follows the system setting.
export function applyTheme(): void {
  const scheme = webApp?.colorScheme ?? (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
  const root = document.documentElement.style;
  root.colorScheme = scheme; // native controls such as the date picker follow it
  for (const [name, value] of Object.entries(colors(scheme, webApp?.themeParams ?? {}))) {
    root.setProperty(`--c-${name}`, value);
  }
}
