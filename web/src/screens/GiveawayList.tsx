import { Plus } from "lucide-react";
import { useEffect } from "react";
import { api, must, type Giveaway, type Me } from "../api/client";
import { useT, type T } from "../i18n";
import { formatEnd } from "../time";
import { Card, ErrorText, Loading, Pager, Pill, ReadOnlyNote, Section, SectionTitle, statusTone, useLoad } from "../ui";

const pageSize = 10;

// giveawayTitle is the admin label, or "Giveaway #<id>" for a giveaway without one.
export function giveawayTitle(g: Giveaway, t: T) {
  return g.admin_label || t("list.untitled", { id: g.id });
}

function GiveawayRow({ g, t, onOpen }: { g: Giveaway; t: T; onOpen: () => void }) {
  // Past tense once the end time has passed: the giveaway is drawing, waits for review or is finished.
  const ended = g.status === "due" || g.status === "review" || g.status === "finished";
  const ends = g.ends_at ? t(ended ? "list.ended" : "list.ends", { time: formatEnd(g.ends_at) }) : t("list.noEnd");
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className="flex min-h-16 w-full items-center gap-3 px-4 py-2 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent"
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate">{giveawayTitle(g, t)}</span>
          <span className="block text-sm text-hint">{ends}</span>
          <span className="block text-sm text-hint">{t("list.participants", { n: g.participants })}</span>
        </span>
        <Pill tone={statusTone(g.status)}>{t(`status.${g.status}`)}</Pill>
      </button>
    </li>
  );
}

// Group is a card of giveaway rows under an optional section caption.
function Group({ title, items, t, onOpen }: { title?: string; items: Giveaway[]; t: T; onOpen: (g: Giveaway) => void }) {
  return (
    <section className="flex flex-col gap-2">
      {title && <SectionTitle>{title}</SectionTitle>}
      <Card className="overflow-hidden">
        <ul className="divide-y divide-hint/20">
          {items.map((g) => (
            <GiveawayRow key={g.id} g={g} t={t} onOpen={() => onOpen(g)} />
          ))}
        </ul>
      </Card>
    </section>
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

function Running({ role, page, onPage, onOpen, onReview }: ListProps) {
  const t = useT();
  const list = useLoad(
    () => must(api.GET("/api/giveaways", { params: { query: pageQuery(page) } })),
    [page],
  );
  useLastPage(list.data?.total, page, onPage);

  if (list.loading && !list.data) return <Loading />;
  if (list.error || !list.data) return <ErrorText error={list.error} onRetry={list.reload} />;

  const { giveaways, review, total } = list.data;
  return (
    <Section>
      {page === 1 && review.length > 0 && (
        <Group title={t("list.review")} items={review} t={t} onOpen={(g) => onReview(g.id)} />
      )}
      {giveaways.length > 0 ? (
        <Group title={t("list.mine")} items={giveaways} t={t} onOpen={(g) => onOpen(g.id)} />
      ) : (
        <Empty>{t("list.empty")}</Empty>
      )}
      <Pager page={page} pages={pageCount(total)} onPage={onPage} />
      {role === "read_only" && <ReadOnlyNote />}
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
      <Group items={list.data.giveaways} t={t} onOpen={(g) => (g.status === "review" ? onReview(g.id) : onOpen(g.id))} />
      <Pager page={page} pages={pageCount(list.data.total)} onPage={onPage} />
    </Section>
  );
}

interface Props {
  role: Me["role"];
  onOpen: (id: number) => void;
  onReview: (id: number) => void;
  onNew: () => void;
}

// ListProps is what one tab needs: its page and the actions on its rows.
type ListProps = Props & { page: number; onPage: (page: number) => void };

type GiveawayListProps = ListProps & { tab: number; onTab: (tab: number) => void };

export default function GiveawayList(props: GiveawayListProps) {
  const t = useT();
  const tabs = [t("list.running"), t("list.archive")];
  return (
    <Section>
      <header className="flex min-h-12 items-center justify-between gap-2">
        <h1 className="truncate text-3xl font-bold">{t("list.title")}</h1>
        {props.role === "full" && (
          <button
            type="button"
            aria-label={t("list.new")}
            title={t("list.new")}
            onClick={props.onNew}
            className="inline-flex size-12 shrink-0 items-center justify-center rounded-full text-accent focus-visible:outline-2 focus-visible:outline-accent"
          >
            <Plus aria-hidden className="size-7" />
          </button>
        )}
      </header>
      {/* A segmented control, as iOS switches between views of one list. */}
      <div role="tablist" className="grid grid-cols-2 rounded-xl bg-hint/15 p-0.5">
        {tabs.map((label, i) => (
          <button
            key={label}
            type="button"
            role="tab"
            id={`tab-${i}`}
            aria-selected={props.tab === i}
            aria-controls="tabpanel"
            onClick={() => props.onTab(i)}
            className={`min-h-12 rounded-[10px] text-sm focus-visible:outline-2 focus-visible:outline-accent ${
              props.tab === i ? "bg-card font-semibold shadow-sm ring-1 ring-hint/20" : "font-medium text-hint"
            }`}
          >
            {label}
          </button>
        ))}
      </div>
      <div role="tabpanel" id="tabpanel" aria-labelledby={`tab-${props.tab}`}>
        {props.tab === 0 ? <Running {...props} /> : <Archive {...props} />}
      </div>
    </Section>
  );
}
