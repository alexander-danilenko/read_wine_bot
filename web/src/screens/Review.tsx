import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText from "@mui/material/ListItemText";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";
import { useState } from "react";
import { api, Failure, must, type Review as ReviewData } from "../api/client";
import { useT, type TextKey } from "../i18n";
import { openLink } from "../telegram";
import { Confirm, ErrorText, Loading, Section, useLoad, useNotify } from "../ui";

type Winner = ReviewData["winners"][number];

const subscriptionText: Record<Winner["subscriptions"][number], TextKey> = {
  subscribed: "review.subscribed",
  not_subscribed: "review.notSubscribed",
  unknown: "review.unknown",
};

const subscriptionColor = { subscribed: "success", not_subscribed: "error", unknown: "default" } as const;

function name(w: Winner): string {
  return [w.first_name, w.last_name].filter(Boolean).join(" ") || (w.username ? `@${w.username}` : String(w.user_id));
}

// Review is the results review: reroll winners, then publish or close. Publish and close
// cannot be undone, so each asks first; a review someone else changed reloads.
export default function Review({ id, onClosed }: { id: number; onClosed: () => void }) {
  const t = useT();
  const notify = useNotify();
  const review = useLoad(() => must(api.GET("/api/reviews/{id}", { params: { path: { id } } })), [id]);
  const [asking, setAsking] = useState<"publish" | "skip">();
  const [busy, setBusy] = useState<number | "close">();
  const [failure, setFailure] = useState<unknown>();

  if (review.loading && !review.data) return <Loading />;
  if (!review.data) return <ErrorText error={review.error} onRetry={review.reload} />;
  const data = review.data;

  const failed = (err: unknown) => {
    setFailure(err);
    if (err instanceof Failure && err.code === "stale") review.reload();
  };

  const reroll = async (w: Winner) => {
    setBusy(w.user_id);
    setFailure(undefined);
    try {
      review.set(await must(api.POST("/api/reviews/{id}/reroll", { params: { path: { id } }, body: { winner_id: w.user_id } })));
      notify(t("review.rerolled"));
    } catch (err) {
      failed(err);
    } finally {
      setBusy(undefined);
    }
  };

  const close = async (action: "publish" | "skip") => {
    setBusy("close");
    setFailure(undefined);
    try {
      await must(api.POST(`/api/reviews/{id}/${action}`, { params: { path: { id } }, body: { rerolled: data.rerolled } }));
      notify(t(action === "publish" ? "review.published" : "review.skipped"));
      onClosed();
    } catch (err) {
      setAsking(undefined);
      failed(err);
    } finally {
      setBusy(undefined);
    }
  };

  return (
    <Section>
      <Typography variant="h6" component="h1">
        {t("review.title")}
      </Typography>
      {failure !== undefined && <ErrorText error={failure} />}
      <Button variant="outlined" size="large" fullWidth onClick={() => openLink(data.giveaway.post_link)}>
        {t("details.post")}
      </Button>
      <Paper>
        {data.winners.length === 0 ? (
          <Typography sx={{ p: 2 }}>{t("review.noWinners")}</Typography>
        ) : (
          <List aria-label={t("review.winners")}>
            {data.winners.map((w) => (
              <ListItem key={w.user_id} sx={{ flexDirection: "column", alignItems: "stretch", gap: 1 }} divider>
                <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}>
                  <ListItemText primary={name(w)} secondary={t("review.place", { n: w.place })} />
                  <Button
                    onClick={() => reroll(w)}
                    loading={busy === w.user_id}
                    disabled={busy !== undefined}
                    aria-label={t("review.rerollLabel", { name: name(w) })}
                  >
                    {t("review.reroll")}
                  </Button>
                </Stack>
                <Stack direction="row" sx={{ flexWrap: "wrap", gap: 1 }}>
                  {w.subscriptions.map((s, i) => (
                    <Chip
                      key={data.channels[i]?.id ?? i}
                      size="small"
                      variant="outlined"
                      color={subscriptionColor[s]}
                      label={`${data.channels[i]?.title ?? ""}: ${t(subscriptionText[s])}`}
                    />
                  ))}
                </Stack>
              </ListItem>
            ))}
          </List>
        )}
      </Paper>
      <Button variant="contained" size="large" fullWidth onClick={() => setAsking("publish")} disabled={busy !== undefined}>
        {t("review.publish")}
      </Button>
      <Button color="error" size="large" fullWidth onClick={() => setAsking("skip")} disabled={busy !== undefined}>
        {t("review.skip")}
      </Button>
      <Confirm
        open={asking === "publish"}
        title={t("review.publishTitle")}
        text={t("review.publishText")}
        confirm={t("review.publish")}
        busy={busy === "close"}
        onConfirm={() => close("publish")}
        onClose={() => setAsking(undefined)}
      />
      <Confirm
        open={asking === "skip"}
        title={t("review.skipTitle")}
        text={t("review.skipText")}
        confirm={t("review.skip")}
        danger
        busy={busy === "close"}
        onConfirm={() => close("skip")}
        onClose={() => setAsking(undefined)}
      />
    </Section>
  );
}
