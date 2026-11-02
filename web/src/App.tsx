import Container from "@mui/material/Container";
import CssBaseline from "@mui/material/CssBaseline";
import Snackbar from "@mui/material/Snackbar";
import { ThemeProvider } from "@mui/material/styles";
import Typography from "@mui/material/Typography";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, must, type Giveaway } from "./api/client";
import { TextContext, translator, type Lang } from "./i18n";
import GiveawayDetails from "./screens/GiveawayDetails";
import GiveawayForm from "./screens/GiveawayForm";
import GiveawayList from "./screens/GiveawayList";
import Review from "./screens/Review";
import { webApp } from "./telegram";
import { themeFrom } from "./theme";
import { ErrorText, Loading, NotifyContext, useLoad } from "./ui";

type Screen =
  | { name: "list" }
  | { name: "new" }
  | { name: "details"; id: number }
  | { name: "edit"; giveaway: Giveaway }
  | { name: "review"; id: number };

// firstScreen opens the review that the bot's "Open review" button names (?review=<id>). The
// parameter only chooses the screen: the API checks who may see it.
function firstScreen(): Screen {
  const id = Number(new URLSearchParams(window.location.search).get("review"));
  return Number.isInteger(id) && id > 0 ? { name: "review", id } : { name: "list" };
}

function useTelegramTheme() {
  const make = () => themeFrom(webApp?.colorScheme ?? "light", webApp?.themeParams ?? {});
  const [theme, setTheme] = useState(make);

  useEffect(() => {
    if (!webApp) return;
    webApp.ready();
    webApp.expand();
    const changed = () => setTheme(make());
    webApp.onEvent("themeChanged", changed);
    return () => webApp?.offEvent("themeChanged", changed);
  }, []);

  return theme;
}

export default function App() {
  const theme = useTelegramTheme();
  const fallbackLang: Lang = navigator.language.startsWith("en") ? "en" : "uk";

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Container component="main" maxWidth="sm" sx={{ py: 2, px: 2, minHeight: "100vh" }}>
        {webApp?.initData ? (
          <Signed fallbackLang={fallbackLang} />
        ) : (
          <Typography sx={{ py: 6, textAlign: "center" }}>{translator(fallbackLang)("app.openFromBot")}</Typography>
        )}
      </Container>
    </ThemeProvider>
  );
}

// Signed is the app for a user Telegram signed in: the screens in the user's language.
function Signed({ fallbackLang }: { fallbackLang: Lang }) {
  const me = useLoad(() => must(api.GET("/api/me")), []);
  const t = useMemo(() => translator(me.data?.language ?? fallbackLang), [me.data, fallbackLang]);
  const [screen, setScreen] = useState<Screen>(firstScreen);
  const [tab, setTab] = useState(0);
  const [notice, setNotice] = useState<string>();

  const back = useCallback(() => {
    setScreen((s) => (s.name === "edit" ? { name: "details", id: s.giveaway.id } : { name: "list" }));
  }, []);

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
      case "list":
        body = (
          <GiveawayList
            tab={tab}
            onTab={setTab}
            onOpen={(id) => setScreen({ name: "details", id })}
            onReview={(id) => setScreen({ name: "review", id })}
            onNew={() => setScreen({ name: "new" })}
          />
        );
        break;
      case "new":
        body = <GiveawayForm onDone={(g) => setScreen({ name: "details", id: g.id })} />;
        break;
      case "edit":
        body = <GiveawayForm giveaway={screen.giveaway} onDone={(g) => setScreen({ name: "details", id: g.id })} />;
        break;
      case "details":
        body = (
          <GiveawayDetails
            key={screen.id}
            id={screen.id}
            onEdit={(giveaway) => setScreen({ name: "edit", giveaway })}
            onReview={() => setScreen({ name: "review", id: screen.id })}
          />
        );
        break;
      case "review":
        body = <Review key={screen.id} id={screen.id} onClosed={() => setScreen({ name: "list" })} />;
        break;
    }
  }

  return (
    <TextContext.Provider value={t}>
      <NotifyContext.Provider value={setNotice}>
        {body}
        <Snackbar
          open={notice !== undefined}
          message={notice}
          autoHideDuration={4000}
          onClose={() => setNotice(undefined)}
          anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
        />
      </NotifyContext.Provider>
    </TextContext.Provider>
  );
}
