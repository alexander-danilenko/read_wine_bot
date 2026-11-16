import { CircleCheck, CircleHelp, CircleX, ExternalLink, RefreshCw, Send } from "lucide-react";
import { useState } from "react";
import { api, Failure, must, type Review as ReviewData } from "../api/client";
import { useT } from "../i18n";
import { openLink } from "../telegram";
import { ActionRow, Button, Card, Confirm, ErrorText, Loading, Section, Title, useLoad, useNotify } from "../ui";

type Winner = ReviewData["winners"][number];

type ReviewChannel = ReviewData["channels"][number];

type Subscription = Winner["subscriptions"][number];

// marks shows a winner's subscription to a channel by icon shape and color, so color is never
// the only cue; unknown means the check failed, which is never shown as not subscribed.
const marks = {
  subscribed: <CircleCheck aria-hidden className="size-4 shrink-0 text-ok" />,
  not_subscribed: <CircleX aria-hidden className="size-4 shrink-0 text-danger" />,
  unknown: <CircleHelp aria-hidden className="size-4 shrink-0 text-hint" />,
};

// ChannelTag is a channel with the winner's subscription mark: a link to it when it has a public
// username, plain text otherwise. A long title is cut short; the link's tap area is 48 px tall.
function ChannelTag({ channel, sub }: { channel: ReviewChannel; sub: Subscription }) {
  const t = useT();
  const state = t(`review.${sub}`);
  const tag = (
    <span className="flex min-w-0 items-center gap-1 rounded-full border border-current px-2.5 py-0.5 text-sm text-hint">
      {marks[sub]}
      <span className="sr-only">{state}:</span>
      <span className="truncate">{channel.title}</span>
      {channel.username && <ExternalLink aria-hidden className="size-3.5 shrink-0" />}
    </span>
  );
  if (!channel.username) return <span className="flex min-h-12 min-w-0 max-w-full items-center">{tag}</span>;
  return (
    <button
      type="button"
      onClick={() => openLink(`https://t.me/${channel.username}`)}
      aria-label={`${state}: ${t("review.openChannel", { name: channel.title })}`}
      className="flex min-h-12 min-w-0 max-w-full items-center rounded-full focus-visible:outline-2 focus-visible:outline-accent"
    >
      {tag}
    </button>
  );
}

function name(w: Winner): string {
  return [w.first_name, w.last_name].filter(Boolean).join(" ") || (w.username ? `@${w.username}` : String(w.user_id));
}

// ChannelTags lists every channel of the review, the giveaway's own first, each with the
// winner's subscription mark.
function ChannelTags({ w, channels }: { w: Winner; channels: ReviewChannel[] }) {
  if (channels.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-x-2">
      {channels.map((c, i) => (
        <ChannelTag key={c.id} channel={c} sub={w.subscriptions[i] ?? "unknown"} />
      ))}
    </div>
  );
}

// Review is the results review: reroll winners, then publish or close. Publish and close
// cannot be undone, so each asks first; a review someone else changed reloads.
export default function Review({ id, onClosed }: { id: number; onClosed: () => void }) {
  const t = useT();
  const notify = useNotify();
  const review = useLoad(() => must(api.GET("/api/reviews/{id}", { params: { path: { id } } })), [id]);
  const [asking, setAsking] = useState<"publish" | "skip">();
  const [busy, setBusy] = useState<number | "close" | "send">();
  const [failure, setFailure] = useState<unknown>();
  // rerolling is the winner a reroll asks about: a reroll cannot be undone.
  const [rerolling, setRerolling] = useState<Winner>();

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
      setRerolling(undefined);
    }
  };

  // sendWinners has the bot message the current winners to the admin, to forward to partners.
  const sendWinners = async () => {
    setBusy("send");
    setFailure(undefined);
    try {
      await must(api.POST("/api/reviews/{id}/send-winners", { params: { path: { id } } }));
      notify(t("review.winnersSent"));
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
      <Title>{t("review.title")}</Title>
      {failure !== undefined && <ErrorText error={failure} />}
      <Card className="overflow-hidden">
        {data.winners.length === 0 ? (
          <p className="p-4">{t("review.noWinners")}</p>
        ) : (
          <ul aria-label={t("review.winners")} className="divide-y divide-hint/20">
            {data.winners.map((w) => (
              <li key={w.user_id} className="flex items-center gap-3 py-2 pr-1 pl-4">
                <span aria-hidden className="flex size-7 shrink-0 items-center justify-center rounded-full bg-accent text-sm font-medium text-on-accent">
                  {w.place}
                </span>
                <span className="sr-only">{t("review.place", { n: w.place })}</span>
                <div className="min-w-0 flex-1">
                  <p className="truncate">{name(w)}</p>
                  <ChannelTags w={w} channels={data.channels} />
                </div>
                {data.can_act && (
                  <button
                    type="button"
                    onClick={() => setRerolling(w)}
                    disabled={busy !== undefined}
                    aria-label={t("review.rerollLabel", { name: name(w) })}
                    className="flex size-12 shrink-0 items-center justify-center rounded-xl text-accent disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-accent"
                  >
                    <RefreshCw aria-hidden className="size-5" />
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>
      <Card className="divide-y divide-hint/20 overflow-hidden">
        <ActionRow icon={<ExternalLink />} onClick={() => openLink(data.giveaway.post_link)}>
          {t("details.post")}
        </ActionRow>
        {data.winners.length > 0 && (
          <ActionRow icon={<Send />} onClick={sendWinners} busy={busy === "send"} disabled={busy !== undefined}>
            {t("review.sendWinners")}
          </ActionRow>
        )}
      </Card>
      {data.can_act && (
        <>
          <Button variant="primary" full onClick={() => setAsking("publish")} disabled={busy !== undefined}>
            {t("review.publish")}
          </Button>
          <Card className="overflow-hidden">
            <ActionRow danger onClick={() => setAsking("skip")} disabled={busy !== undefined}>
              {t("review.skip")}
            </ActionRow>
          </Card>
        </>
      )}
      <Confirm
        open={rerolling !== undefined}
        title={rerolling ? t("review.rerollTitle", { name: name(rerolling) }) : ""}
        text={t("review.rerollText")}
        confirm={t("review.reroll")}
        busy={rerolling !== undefined && busy === rerolling.user_id}
        onConfirm={() => rerolling && reroll(rerolling)}
        onClose={() => setRerolling(undefined)}
      />
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
