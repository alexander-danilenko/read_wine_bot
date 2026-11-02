import { CircleHelp, Settings } from "lucide-react";
import { useEffect, type ReactNode } from "react";
import { api, must, type Channel, type Giveaway } from "../api/client";
import { useT, type T } from "../i18n";
import { formatEnd } from "../time";
import { Avatar, Button, Card, ErrorText, Loading, Pager, Pill, Section, statusTone, useLoad } from "../ui";

const pageSize = 10;

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

// maxOffset is the largest offset the API accepts. A page beyond it asks for the last offset,
// gets no rows and moves to the last page, instead of failing.
const maxOffset = 100000;

// pageQuery asks the API for one page of a list.
function pageQuery(page: number) {
  return { offset: Math.min((page - 1) * pageSize, maxOffset), limit: pageSize };
}

// pageCount is how many pages a list of total giveaways fills, at least one.
function pageCount(total: number) {
  return Math.max(1, Math.ceil(total / pageSize));
}

// useLastPage moves to the last page when the asked page is past the end, for example after a
// cancel or restore shrank the list. It compares with the total, not the rows shown, so the
// previous page's data, still on screen while the next page loads, cannot trigger it again.
function useLastPage(total: number | undefined, page: number, onPage: (page: number) => void) {
  useEffect(() => {
    if (total !== undefined && page > pageCount(total)) onPage(pageCount(total));
  }, [total, page, onPage]);
}

function Running({ page, onPage, onOpen, onReview, onNew }: ListProps) {
  const t = useT();
  const list = useLoad(
    () => must(api.GET("/api/giveaways", { params: { query: pageQuery(page) } })),
    [page],
  );
  useLastPage(list.data?.total, page, onPage);

  if (list.loading && !list.data) return <Loading />;
  if (list.error || !list.data) return <ErrorText error={list.error} onRetry={list.reload} />;

  const { giveaways, review, channels, total } = list.data;
  return (
    <Section>
      {page === 1 && review.length > 0 && (
        <Group title={t("list.review")} items={review} channels={channels} t={t} onOpen={(g) => onReview(g.id)} />
      )}
      {giveaways.length > 0 ? (
        <Group title={t("list.mine")} items={giveaways} channels={channels} t={t} onOpen={(g) => onOpen(g.id)} />
      ) : (
        <Empty>{t("list.empty")}</Empty>
      )}
      <Pager page={page} pages={pageCount(total)} onPage={onPage} />
      <Button variant="primary" full onClick={onNew}>
        <svg aria-hidden viewBox="0 0 24 24" className="size-5 fill-current">
          <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" />
        </svg>
        {t("list.new")}
      </Button>
    </Section>
  );
}

function Archive({ page, onPage, onOpen, onReview }: ListProps) {
  const t = useT();
  const list = useLoad(
    () => must(api.GET("/api/giveaways/finished", { params: { query: pageQuery(page) } })),
    [page],
  );
  useLastPage(list.data?.total, page, onPage);

  if (list.loading && !list.data) return <Loading />;
  if (list.error || !list.data) return <ErrorText error={list.error} onRetry={list.reload} />;
  if (list.data.giveaways.length === 0) return <Empty>{t("list.archiveEmpty")}</Empty>;

  return (
    <Section>
      <Group items={list.data.giveaways} channels={list.data.channels} t={t} onOpen={(g) => (g.status === "review" ? onReview(g.id) : onOpen(g.id))} />
      <Pager page={page} pages={pageCount(list.data.total)} onPage={onPage} />
    </Section>
  );
}

interface Props {
  onOpen: (id: number) => void;
  onReview: (id: number) => void;
  onNew: () => void;
}

// ListProps is what one tab needs: its page and the actions on its rows.
type ListProps = Props & { page: number; onPage: (page: number) => void };

// HeaderIcon is a 48 px icon button of the list header; label names it for screen readers.
function HeaderIcon({ label, onClick, children }: { label: string; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className="inline-flex size-12 items-center justify-center rounded-xl text-accent focus-visible:outline-2 focus-visible:outline-accent"
    >
      {children}
    </button>
  );
}

type GiveawayListProps = ListProps & {
  tab: number;
  onTab: (tab: number) => void;
  onHelp: () => void;
  onSettings: () => void;
};

export default function GiveawayList(props: GiveawayListProps) {
  const t = useT();
  const tabs = [t("list.running"), t("list.archive")];
  return (
    <Section>
      <header className="flex items-end gap-1 border-b border-hint/30">
        <div role="tablist" className="grid flex-1 grid-cols-2">
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
        <nav aria-label={t("nav.label")} className="flex">
          <HeaderIcon label={t("nav.help")} onClick={props.onHelp}>
            <CircleHelp aria-hidden className="size-6" />
          </HeaderIcon>
          <HeaderIcon label={t("nav.settings")} onClick={props.onSettings}>
            <Settings aria-hidden className="size-6" />
          </HeaderIcon>
        </nav>
      </header>
      <div role="tabpanel" id="tabpanel" aria-labelledby={`tab-${props.tab}`}>
        {props.tab === 0 ? <Running {...props} /> : <Archive {...props} />}
      </div>
    </Section>
  );
}
