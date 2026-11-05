import { ExternalLink, Send } from "lucide-react";
import { useState } from "react";
import { api, Failure, must, type Review as ReviewData } from "../api/client";
import { useT } from "../i18n";
import { openLink } from "../telegram";
import { Button, Card, Confirm, ErrorText, Loading, Section, Title, useLoad, useNotify } from "../ui";

type Winner = ReviewData["winners"][number];

type ReviewChannel = ReviewData["channels"][number];

// ChannelTag is a channel the winner is subscribed to: a link to it when it has a public
// username, plain text otherwise. A long title is cut short; the link's tap area is 48 px tall.
function ChannelTag({ channel }: { channel: ReviewChannel }) {
  const t = useT();
  const tag = (
    <span className="flex min-w-0 items-center gap-1 rounded-full border border-current px-2.5 py-0.5 text-sm text-hint">
      <span className="truncate">{channel.title}</span>
      {channel.username && <ExternalLink aria-hidden className="size-3.5 shrink-0" />}
    </span>
  );
  if (!channel.username) return <span className="flex min-h-12 min-w-0 max-w-full items-center">{tag}</span>;
  return (
    <button
      type="button"
      onClick={() => openLink(`https://t.me/${channel.username}`)}
      aria-label={t("review.openChannel", { name: channel.title })}
      className="flex min-h-12 min-w-0 max-w-full items-center rounded-full focus-visible:outline-2 focus-visible:outline-accent"
    >
      {tag}
    </button>
  );
}

// subscribedChannels returns the channels the winner is confirmed subscribed to; channels
// they are not subscribed to, or whose check failed, are left out.
function subscribedChannels(w: Winner, channels: ReviewChannel[]): ReviewChannel[] {
  return channels.filter((_, i) => w.subscriptions[i] === "subscribed");
}

function name(w: Winner): string {
  return [w.first_name, w.last_name].filter(Boolean).join(" ") || (w.username ? `@${w.username}` : String(w.user_id));
}

// ChannelTags lists a winner's channels; a winner subscribed to none shows nothing.
function ChannelTags({ channels }: { channels: ReviewChannel[] }) {
  if (channels.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-x-2">
      {channels.map((c) => (
        <ChannelTag key={c.id} channel={c} />
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
      <Button variant="outline" full onClick={() => openLink(data.giveaway.post_link)}>
        {t("details.post")}
      </Button>
      <Card className="overflow-hidden">
        {data.winners.length === 0 ? (
          <p className="p-4">{t("review.noWinners")}</p>
        ) : (
          <ul aria-label={t("review.winners")} className="divide-y divide-hint/20">
            {data.winners.map((w) => (
              <li key={w.user_id} className="flex flex-col gap-2 px-4 py-3">
                <div className="flex items-center gap-2">
                  <div className="min-w-0 flex-1">
                    <p className="truncate">{name(w)}</p>
                    <p className="text-sm text-hint">{t("review.place", { n: w.place })}</p>
                  </div>
                  <Button
                    onClick={() => reroll(w)}
                    busy={busy === w.user_id}
                    disabled={busy !== undefined}
                    aria-label={t("review.rerollLabel", { name: name(w) })}
                  >
                    {t("review.reroll")}
                  </Button>
                </div>
                <ChannelTags channels={subscribedChannels(w, data.channels)} />
              </li>
            ))}
          </ul>
        )}
      </Card>
      {data.winners.length > 0 && (
        <Button variant="outline" full onClick={sendWinners} busy={busy === "send"} disabled={busy !== undefined}>
          <Send aria-hidden className="size-5" />
          {t("review.sendWinners")}
        </Button>
      )}
      <Button variant="primary" full onClick={() => setAsking("publish")} disabled={busy !== undefined}>
        {t("review.publish")}
      </Button>
      <Button variant="danger-text" full onClick={() => setAsking("skip")} disabled={busy !== undefined}>
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
