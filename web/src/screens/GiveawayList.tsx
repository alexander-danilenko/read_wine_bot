import { useState } from "react";
import { api, must, type Channel, type Giveaway } from "../api/client";
import { useT, type T } from "../i18n";
import { formatEnd } from "../time";
import { Avatar, Button, Card, ErrorText, Loading, Pill, Section, statusTone, useLoad } from "../ui";

const pageSize = 25;

function GiveawayRow({ g, channel, t, onOpen }: { g: Giveaway; channel?: Channel; t: T; onOpen: () => void }) {
  const ends = g.ends_at ? t("list.ends", { time: formatEnd(g.ends_at) }) : t("list.noEnd");
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className="flex min-h-16 w-full items-center gap-3 px-4 py-2 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent"
      >
        <Avatar channel={channel} />
        <span className="min-w-0 flex-1">
          {/* The post link stands in when Telegram cannot give the channel title. */}
          <span className="block truncate">{channel?.title || g.post_link}</span>
          <span className="block text-sm text-hint">{`${ends} · ${t("list.participants", { n: g.participants })}`}</span>
        </span>
        <Pill tone={statusTone(g.status)}>{t(`status.${g.status}`)}</Pill>
      </button>
    </li>
  );
}

// Group is a card of giveaway rows under an optional heading.
function Group(props: { title?: string; items: Giveaway[]; channels: Channel[]; t: T; onOpen: (g: Giveaway) => void }) {
  const { title, items, t, onOpen } = props;
  const channels = new Map(props.channels.map((c) => [c.id, c]));
  return (
    <Card className="overflow-hidden">
      {title && <h2 className="px-4 pt-3 text-sm font-medium text-hint">{title}</h2>}
      <ul className="divide-y divide-hint/20">
        {items.map((g) => (
          <GiveawayRow key={g.id} g={g} channel={channels.get(g.chat_id)} t={t} onOpen={() => onOpen(g)} />
        ))}
      </ul>
    </Card>
  );
}

function Empty({ children }: { children: string }) {
  return <p className="py-6 text-center text-hint">{children}</p>;
}

function Running({ onOpen, onReview, onNew }: Props) {
  const t = useT();
  const list = useLoad(() => must(api.GET("/api/giveaways")), []);

  if (list.loading && !list.data) return <Loading />;
  if (list.error || !list.data) return <ErrorText error={list.error} onRetry={list.reload} />;

  const { giveaways, review, channels } = list.data;
  return (
    <Section>
      {review.length > 0 && <Group title={t("list.review")} items={review} channels={channels} t={t} onOpen={(g) => onReview(g.id)} />}
      {giveaways.length > 0 ? (
        <Group title={t("list.mine")} items={giveaways} channels={channels} t={t} onOpen={(g) => onOpen(g.id)} />
      ) : (
        <Empty>{t("list.empty")}</Empty>
      )}
      <Button variant="primary" full onClick={onNew}>
        <svg aria-hidden viewBox="0 0 24 24" className="size-5 fill-current">
          <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" />
        </svg>
        {t("list.new")}
      </Button>
    </Section>
  );
}

function Finished({ onOpen, onReview }: Props) {
  const t = useT();
  const [pages, setPages] = useState(1);
  const list = useLoad(
    () => must(api.GET("/api/giveaways/finished", { params: { query: { limit: pages * pageSize } } })),
    [pages],
  );

  if (list.loading && !list.data) return <Loading />;
  if (list.error || !list.data) return <ErrorText error={list.error} onRetry={list.reload} />;
  if (list.data.giveaways.length === 0) return <Empty>{t("list.finishedEmpty")}</Empty>;

  return (
    <Section>
      <Group items={list.data.giveaways} channels={list.data.channels} t={t} onOpen={(g) => (g.status === "review" ? onReview(g.id) : onOpen(g.id))} />
      {list.data.more && (
        <Button onClick={() => setPages((p) => p + 1)} busy={list.loading}>
          {t("list.more")}
        </Button>
      )}
    </Section>
  );
}

interface Props {
  onOpen: (id: number) => void;
  onReview: (id: number) => void;
  onNew: () => void;
}

export default function GiveawayList(props: Props & { tab: number; onTab: (tab: number) => void }) {
  const t = useT();
  const tabs = [t("list.running"), t("list.finished")];
  return (
    <Section>
      <div role="tablist" className="grid grid-cols-2 border-b border-hint/30">
        {tabs.map((label, i) => (
          <button
            key={label}
            type="button"
            role="tab"
            id={`tab-${i}`}
            aria-selected={props.tab === i}
            aria-controls="tabpanel"
            onClick={() => props.onTab(i)}
            className={`-mb-px min-h-12 border-b-2 font-medium focus-visible:outline-2 focus-visible:outline-accent ${
              props.tab === i ? "border-accent text-accent" : "border-transparent text-hint"
            }`}
          >
            {label}
          </button>
        ))}
      </div>
      <div role="tabpanel" id="tabpanel" aria-labelledby={`tab-${props.tab}`}>
        {props.tab === 0 ? <Running {...props} /> : <Finished {...props} />}
      </div>
    </Section>
  );
}
