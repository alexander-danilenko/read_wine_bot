// The parts of Telegram's Mini App script (telegram-web-app.js) the app uses.
interface TelegramWebApp {
  initData: string;
}

interface Window {
  Telegram?: { WebApp?: TelegramWebApp };
}
