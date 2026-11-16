import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, must, onRoleLost, type Giveaway, type Me } from "./api/client";
import { TextContext, translator, type Lang } from "./i18n";
import GiveawayDetails from "./screens/GiveawayDetails";
import GiveawayForm from "./screens/GiveawayForm";
import GiveawayList from "./screens/GiveawayList";
import Partner from "./screens/Partner";
import Review from "./screens/Review";
import Settings from "./screens/Settings";
import { webApp } from "./telegram";
import { applyTheme } from "./theme";
import { Confirm, ErrorText, Loading, NotifyContext, TabBar, Toast, useLoad, type Tab } from "./ui";

// ListAt is a place in the giveaway list: the tab (0 Running, 1 Archive) and its page.
type ListAt = { tab: number; page: number };

const home: ListAt = { tab: 0, page: 1 };

// The list and settings are the two sections' first screens. The other giveaway screens keep the
// list place they were opened from, so back returns there; the partner preview returns to settings.
type Screen =
  | { name: "list"; at: ListAt }
  | { name: "settings" }
  | { name: "partner" }
  | { name: "new"; from: ListAt }
  | { name: "details"; id: number; from: ListAt }
  | { name: "edit"; id: number; from: ListAt }
  | { name: "review"; id: number; from: ListAt };

// bareScreens are the screens named by a query key alone, with no value: ?new, ?settings,
// ?partner (the partner screen as a primary admin previews it).
const bareScreens = ["new", "settings", "partner"] as const;

// positive is the whole number above 0 that text holds, or undefined.
function positive(text: string | null): number | undefined {
  return text !== null && /^[1-9]\d{0,9}$/.test(text) ? Number(text) : undefined;
}

// parseScreen reads the screen from the URL's query string, which screenURL writes and the bot's
// buttons set, such as "Review in the app" (?review=<id>). A parameter only chooses the screen:
// the API
// checks who may see it. Anything malformed opens the first page of Running.
function parseScreen(search: string): Screen {
  const q = new URLSearchParams(search);
  const fallback: Screen = { name: "list", at: home };

  for (const [key, name] of [["review", "review"], ["giveaway", "details"], ["edit", "edit"]] as const) {
    if (!q.has(key)) continue;
    const id = positive(q.get(key));
    return id ? { name, id, from: home } : fallback;
  }
  for (const name of bareScreens) {
    if (q.has(name)) return name === "new" ? { name, from: home } : { name };
  }

  const tab = q.get("tab");
  const page = q.has("page") ? positive(q.get("page")) : 1;
  if ((tab !== null && tab !== "archive") || !page) return fallback;
  return { name: "list", at: { tab: tab === "archive" ? 1 : 0, page } };
}

// allowedScreen is screen as role allows it, or screen itself when it is allowed. Only a full admin
// creates or edits: for any other role a new screen falls back to the list it was opened from,
// and an edit screen to that giveaway's details. A partner sees one screen whatever the URL
// says, so their screen is the list, which has no back button.
function allowedScreen(screen: Screen, role: Me["role"]): Screen {
  if (role === "partner") return screen.name === "list" ? screen : { name: "list", at: home };
  if (role === "full") return screen;
  if (screen.name === "new") return { name: "list", at: screen.from };
  if (screen.name === "edit") return { name: "details", id: screen.id, from: screen.from };
  return screen;
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
  const query = (bareScreens as readonly string[]).includes(screen.name) ? screen.name : q.toString();
  return `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`;
}

// parent is the screen Telegram's back button returns to, or undefined on a section's first screen.
function parent(screen: Screen): Screen | undefined {
  switch (screen.name) {
    case "list":
    case "settings":
      return undefined;
    case "partner":
      return { name: "settings" };
    case "edit":
      return { name: "details", id: screen.id, from: screen.from };
    default:
      return { name: "list", at: screen.from };
  }
}

// isField reports whether target takes typing, so the tab bar can step aside for the keyboard.
function isField(target: EventTarget): boolean {
  return target instanceof HTMLElement && target.matches("input:not([readonly]), select, textarea");
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
function EditScreen({ id, onDone, onDirty }: { id: number; onDone: (g: Giveaway) => void; onDirty: (dirty: boolean) => void }) {
  const g = useLoad(() => must(api.GET("/api/giveaways/{id}", { params: { path: { id } } })), [id]);
  if (g.loading && !g.data) return <Loading />;
  if (g.error || !g.data) return <ErrorText error={g.error} onRetry={g.reload} />;
  return <GiveawayForm giveaway={g.data.giveaway} onDone={onDone} onDirty={onDirty} />;
}

// Signed is the app for a user Telegram signed in: the screens in the user's language.
function Signed({ fallbackLang }: { fallbackLang: Lang }) {
  const me = useLoad(() => must(api.GET("/api/me")), []);
  const t = useMemo(() => translator(me.data?.language ?? fallbackLang), [me.data, fallbackLang]);
  const [screen, setScreen] = useState<Screen>(() => parseScreen(window.location.search));
  const [notice, setNotice] = useState<string>();
  const hideNotice = useCallback(() => setNotice(undefined), []);
  // dirty is true while a giveaway form holds unsaved input; leaving it then asks first.
  const dirty = useRef(false);
  const onDirty = useCallback((d: boolean) => (dirty.current = d), []);
  const [pending, setPending] = useState<Screen>();
  const [typing, setTyping] = useState(false);

  const leave = useCallback((to: Screen) => (dirty.current ? setPending(to) : setScreen(to)), []);

  // A call the caller's role does not allow shows why and reloads the caller, so the screen
  // redraws for the role they have now.
  useEffect(
    () =>
      onRoleLost((code) => {
        setNotice(t(`error.${code}`));
        me.reload();
      }),
    // me.reload only bumps a counter, so the copy from any render works.
    [t],
  );

  // Setting state while rendering redraws at once, before the screen the role forbids shows.
  const allowed = me.data ? allowedScreen(screen, me.data.role) : screen;
  if (allowed !== screen) setScreen(allowed);

  // The URL follows the screen, so Telegram's "Reload page" reopens it. Replacing the entry
  // adds no history: Telegram's back button moves between screens.
  useEffect(() => {
    window.history.replaceState(null, "", screenURL(screen));
  }, [screen]);

  // Telegram's own back button moves between screens; a section's first screen has none.
  useEffect(() => {
    if (!webApp) return;
    const to = parent(screen);
    if (!to) {
      webApp.BackButton.hide();
      return;
    }
    const back = () => leave(to);
    webApp.BackButton.show();
    webApp.BackButton.onClick(back);
    return () => webApp?.BackButton.offClick(back);
  }, [screen, leave]);

  // A field removed by a screen change sends no blur, so read where focus is after every change.
  useEffect(() => {
    window.scrollTo(0, 0);
    setTyping(document.activeElement !== null && isField(document.activeElement));
  }, [screen, me.loading]);

  let body;
  // While the caller reloads, the screen leaves, so it loads again for the role that comes back.
  if (me.loading) body = <Loading />;
  else if (me.error || !me.data) body = <ErrorText error={me.error} onRetry={me.reload} />;
  else if (me.data.role === "partner") body = <Partner me={me.data} />;
  else {
    switch (screen.name) {
      case "list": {
        const at = screen.at;
        body = (
          <GiveawayList
            role={me.data.role}
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
        body = <GiveawayForm onDone={(g) => setScreen({ name: "details", id: g.id, from })} onDirty={onDirty} />;
        break;
      }
      case "edit": {
        const from = screen.from;
        body = (
          <EditScreen key={screen.id} id={screen.id} onDone={(g) => setScreen({ name: "details", id: g.id, from })} onDirty={onDirty} />
        );
        break;
      }
      case "details": {
        const { id, from } = screen;
        body = (
          <GiveawayDetails
            key={id}
            id={id}
            role={me.data.role}
            onEdit={(g) => setScreen({ name: "edit", id: g.id, from })}
            onReview={() => setScreen({ name: "review", id, from })}
          />
        );
        break;
      }
      case "settings":
        body = <Settings me={me.data} onMe={me.set} onPartner={() => setScreen({ name: "partner" })} />;
        break;
      case "partner":
        body = <Partner me={me.data} />;
        break;
      case "review": {
        const from = screen.from;
        body = <Review key={screen.id} id={screen.id} onClosed={() => setScreen({ name: "list", at: from })} />;
        break;
      }
    }
  }

  // Partners have one screen, so only admins get the tab bar. It hides while a field has focus,
  // as the keyboard covers it on iOS, so on Android it never sits over the field being typed in.
  const showTabs = me.data && me.data.role !== "partner";
  const tab: Tab = screen.name === "settings" || screen.name === "partner" ? "settings" : "giveaways";

  return (
    <TextContext.Provider value={t}>
      <NotifyContext.Provider value={setNotice}>
        <div
          className={showTabs ? "pb-[calc(4.5rem_+_var(--safe-bottom))]" : ""}
          onFocus={(e) => setTyping(isField(e.target))}
          onBlur={() => setTyping(false)}
        >
          {body}
        </div>
        {showTabs && !typing && (
          <TabBar current={tab} onTab={(to) => leave(to === "settings" ? { name: "settings" } : { name: "list", at: home })} />
        )}
        <Confirm
          open={pending !== undefined}
          title={t("form.discardTitle")}
          text={t("form.discardText")}
          confirm={t("form.discard")}
          danger
          onConfirm={() => {
            dirty.current = false;
            if (pending) setScreen(pending);
            setPending(undefined);
          }}
          onClose={() => setPending(undefined)}
        />
        <Toast message={notice} onDone={hideNotice} />
      </NotifyContext.Provider>
    </TextContext.Provider>
  );
}
