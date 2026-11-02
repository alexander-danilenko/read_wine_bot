// The parts of Telegram's Mini App script (https://telegram.org/js/telegram-web-app.js) the app uses.
export interface ThemeParams {
  bg_color?: string;
  secondary_bg_color?: string;
  section_bg_color?: string;
  text_color?: string;
  hint_color?: string;
  link_color?: string;
  button_color?: string;
  button_text_color?: string;
  destructive_text_color?: string;
}

interface BackButton {
  show(): void;
  hide(): void;
  onClick(cb: () => void): void;
  offClick(cb: () => void): void;
}

interface WebApp {
  initData: string;
  colorScheme: "light" | "dark";
  themeParams: ThemeParams;
  BackButton: BackButton;
  ready(): void;
  expand(): void;
  openTelegramLink(url: string): void;
  onEvent(event: "themeChanged", cb: () => void): void;
  offEvent(event: "themeChanged", cb: () => void): void;
}

declare global {
  interface Window {
    Telegram?: { WebApp?: WebApp };
  }
}

// webApp is Telegram's Mini App object, or undefined outside Telegram.
export const webApp: WebApp | undefined = window.Telegram?.WebApp;

// openLink opens a t.me link inside Telegram, or in a new tab elsewhere.
export function openLink(url: string): void {
  if (webApp && url.startsWith("https://t.me/")) {
    webApp.openTelegramLink(url);
  } else {
    window.open(url, "_blank", "noopener");
  }
}
