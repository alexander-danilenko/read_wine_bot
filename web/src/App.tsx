import { useCallback, useEffect, useMemo, useState } from "react";
import { api, must, type Giveaway } from "./api/client";
import { TextContext, translator, type Lang } from "./i18n";
import GiveawayDetails from "./screens/GiveawayDetails";
import GiveawayForm from "./screens/GiveawayForm";
import GiveawayList from "./screens/GiveawayList";
import Review from "./screens/Review";
import { webApp } from "./telegram";
import { applyTheme } from "./theme";
import { ErrorText, Loading, NotifyContext, Toast, useLoad } from "./ui";

// ListAt is a place in the giveaway list: the tab (0 Running, 1 Archive) and its page.
type ListAt = { tab: number; page: number };

const home: ListAt = { tab: 0, page: 1 };

// Every screen but the list keeps the list place it was opened from, so back returns there.
type Screen =
  | { name: "list"; at: ListAt }
  | { name: "new"; from: ListAt }
  | { name: "details"; id: number; from: ListAt }
  | { name: "edit"; id: number; from: ListAt }
  | { name: "review"; id: number; from: ListAt };

// positive is the whole number above 0 that text holds, or undefined.
function positive(text: string | null): number | undefined {
  return text !== null && /^[1-9]\d{0,9}$/.test(text) ? Number(text) : undefined;
}

// parseScreen reads the screen from the URL's query string, which screenURL writes and the bot's
// "Review in the app" button sets (?review=<id>). A parameter only chooses the screen: the API
// checks who may see it. Anything malformed opens the first page of Running.
function parseScreen(search: string): Screen {
  const q = new URLSearchParams(search);
  const fallback: Screen = { name: "list", at: home };

  for (const [key, name] of [["review", "review"], ["giveaway", "details"], ["edit", "edit"]] as const) {
    if (!q.has(key)) continue;
    const id = positive(q.get(key));
    return id ? { name, id, from: home } : fallback;
  }
  if (q.has("new")) return { name: "new", from: home };

  const tab = q.get("tab");
  const page = q.has("page") ? positive(q.get("page")) : 1;
  if ((tab !== null && tab !== "archive") || !page) return fallback;
  return { name: "list", at: { tab: tab === "archive" ? 1 : 0, page } };
}

// screenURL is the address of screen. It keeps the hash, where Telegram passes its launch data.
function screenURL(screen: Screen): string {
  const q = new URLSearchParams();
  switch (screen.name) {
    case "list":
      if (screen.at.tab === 1) q.set("tab", "archive");
      if (screen.at.page > 1) q.set("page", String(screen.at.page));
      break;
    case "details":
      q.set("giveaway", String(screen.id));
      break;
    case "edit":
    case "review":
      q.set(screen.name, String(screen.id));
      break;
  }
  const query = screen.name === "new" ? "new" : q.toString();
  return `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`;
}

// useTelegramTheme repaints the page when the user switches Telegram's theme or dark mode.
function useTelegramTheme() {
  useEffect(() => {
    if (!webApp) return;
    webApp.ready();
    webApp.expand();
    webApp.onEvent("themeChanged", applyTheme);
    return () => webApp?.offEvent("themeChanged", applyTheme);
  }, []);
}

export default function App() {
  useTelegramTheme();
  const fallbackLang: Lang = navigator.language.startsWith("en") ? "en" : "uk";

  return (
    <main className="mx-auto min-h-screen max-w-xl p-4">
      {webApp?.initData ? (
        <Signed fallbackLang={fallbackLang} />
      ) : (
        <p className="py-12 text-center">{translator(fallbackLang)("app.openFromBot")}</p>
      )}
    </main>
  );
}

// EditScreen loads the giveaway by id, so the edit form also opens from its own URL.
function EditScreen({ id, onDone }: { id: number; onDone: (g: Giveaway) => void }) {
  const g = useLoad(() => must(api.GET("/api/giveaways/{id}", { params: { path: { id } } })), [id]);
  if (g.loading && !g.data) return <Loading />;
  if (g.error || !g.data) return <ErrorText error={g.error} onRetry={g.reload} />;
  return <GiveawayForm giveaway={g.data.giveaway} onDone={onDone} />;
}

// Signed is the app for a user Telegram signed in: the screens in the user's language.
function Signed({ fallbackLang }: { fallbackLang: Lang }) {
  const me = useLoad(() => must(api.GET("/api/me")), []);
  const t = useMemo(() => translator(me.data?.language ?? fallbackLang), [me.data, fallbackLang]);
  const [screen, setScreen] = useState<Screen>(() => parseScreen(window.location.search));
  const [notice, setNotice] = useState<string>();
  const hideNotice = useCallback(() => setNotice(undefined), []);

  const back = useCallback(() => {
    setScreen((s) => {
      if (s.name === "list") return s;
      if (s.name === "edit") return { name: "details", id: s.id, from: s.from };
      return { name: "list", at: s.from };
    });
  }, []);

  // The URL follows the screen, so Telegram's "Reload page" reopens it. Replacing the entry
  // adds no history: Telegram's back button moves between screens.
  useEffect(() => window.history.replaceState(null, "", screenURL(screen)), [screen]);

  // Telegram's own back button moves between screens; the list has none.
  useEffect(() => {
    if (!webApp) return;
    if (screen.name === "list") {
      webApp.BackButton.hide();
      return;
    }
    webApp.BackButton.show();
    webApp.BackButton.onClick(back);
    return () => webApp?.BackButton.offClick(back);
  }, [screen, back]);

  useEffect(() => window.scrollTo(0, 0), [screen]);

  let body;
  if (me.loading) body = <Loading />;
  else if (me.error) body = <ErrorText error={me.error} onRetry={me.reload} />;
  else {
    switch (screen.name) {
      case "list": {
        const at = screen.at;
        body = (
          <GiveawayList
            tab={at.tab}
            onTab={(tab) => setScreen({ name: "list", at: { tab, page: 1 } })}
            page={at.page}
            onPage={(page) => setScreen({ name: "list", at: { ...at, page } })}
            onOpen={(id) => setScreen({ name: "details", id, from: at })}
            onReview={(id) => setScreen({ name: "review", id, from: at })}
            onNew={() => setScreen({ name: "new", from: at })}
          />
        );
        break;
      }
      case "new": {
        const from = screen.from;
        body = <GiveawayForm onDone={(g) => setScreen({ name: "details", id: g.id, from })} />;
        break;
      }
      case "edit": {
        const from = screen.from;
        body = <EditScreen key={screen.id} id={screen.id} onDone={(g) => setScreen({ name: "details", id: g.id, from })} />;
        break;
      }
      case "details": {
        const { id, from } = screen;
        body = (
          <GiveawayDetails
            key={id}
            id={id}
            onEdit={(g) => setScreen({ name: "edit", id: g.id, from })}
            onReview={() => setScreen({ name: "review", id, from })}
          />
        );
        break;
      }
      case "review": {
        const from = screen.from;
        body = <Review key={screen.id} id={screen.id} onClosed={() => setScreen({ name: "list", at: from })} />;
        break;
      }
    }
  }

  return (
    <TextContext.Provider value={t}>
      <NotifyContext.Provider value={setNotice}>
        {body}
        <Toast message={notice} onDone={hideNotice} />
      </NotifyContext.Provider>
    </TextContext.Provider>
  );
}
