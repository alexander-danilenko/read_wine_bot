import { ChevronRight, ExternalLink } from "lucide-react";
import { useState } from "react";
import { api, Failure, must, type Giveaway, type Me } from "../api/client";
import { useT, type TextKey } from "../i18n";
import { openLink } from "../telegram";
import { formatEnd } from "../time";
import { ActionRow, Button, Card, Confirm, Dialog, ErrorText, Field, Loading, Pill, ReadOnlyNote, Row, Section, statusTone, useLoad, useNotify } from "../ui";
import { giveawayTitle } from "./GiveawayList";

type Action = "cancel" | "restore" | "finish-now";

export default function GiveawayDetails(props: { id: number; role: Me["role"]; onEdit: (g: Giveaway) => void; onReview: () => void }) {
  const t = useT();
  const notify = useNotify();
  const g = useLoad(() => must(api.GET("/api/giveaways/{id}", { params: { path: { id: props.id } } })), [props.id]);
  const [asking, setAsking] = useState<Action | "move">();
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<unknown>();
  const [link, setLink] = useState("");

  if (g.loading && !g.data) return <Loading />;
  if (!g.data) return <ErrorText error={g.error} onRetry={g.reload} />;
  const { giveaway, channel } = g.data;

  // run calls an action and shows its result; a stale giveaway reloads (someone changed it).
  const run = async (call: () => Promise<Giveaway>, done: TextKey) => {
    setBusy(true);
    setFailure(undefined);
    try {
      g.set({ giveaway: await call(), channel }); // actions return the giveaway only
      setAsking(undefined);
      setLink("");
      notify(t(done));
    } catch (err) {
      setFailure(err);
      if (err instanceof Failure && err.code === "stale") g.reload();
      if (!(err instanceof Failure && err.field)) setAsking(undefined);
    } finally {
      setBusy(false);
    }
  };

  const act = (action: Action, done: TextKey) =>
    run(() => must(api.POST(`/api/giveaways/{id}/${action}`, { params: { path: { id: giveaway.id } } })), done);

  const move = () =>
    run(
      () =>
        must(api.POST("/api/giveaways/{id}/move", { params: { path: { id: giveaway.id } }, body: { post_link: link } })),
      "details.moved",
    );

  // The server says whether the caller may change this giveaway; without it no change control shows.
  const running = giveaway.can_change && giveaway.status === "active";
  const cancelled = giveaway.can_change && giveaway.status === "cancelled";
  const changeable = running || cancelled;
  const moveError = asking === "move" && failure instanceof Failure && failure.field ? failure : undefined;

  return (
    <Section>
      <div className="flex items-center justify-between gap-2">
        {/* Wraps instead of cutting a long label short: this is where it can be read whole. */}
        <h1 className="min-w-0 break-words text-xl font-semibold">{giveawayTitle(giveaway, t)}</h1>
        <Pill tone={statusTone(giveaway.status)}>{t(`status.${giveaway.status}`)}</Pill>
      </div>
      {failure !== undefined && !moveError && <ErrorText error={failure} />}
      {props.role === "read_only" && <ReadOnlyNote />}
      <Card className="divide-y divide-hint/20">
        <Row label={t("details.ends")}>
          <span className="truncate">{giveaway.ends_at ? formatEnd(giveaway.ends_at) : t("list.noEnd")}</span>
        </Row>
        <Row label={t("details.participants")}>{giveaway.participants}</Row>
        <Row label={t("form.winners")}>{giveaway.winners}</Row>
        <Row label={t("form.buttonText")}>
          <span className="truncate">{giveaway.button_text}</span>
        </Row>
      </Card>
      {giveaway.status === "review" && (
        <Button variant="primary" full onClick={props.onReview}>
          {t("details.openReview")}
        </Button>
      )}
      {cancelled && (
        <Button variant="primary" full busy={busy} onClick={() => act("restore", "details.restored")}>
          {t("details.restore")}
        </Button>
      )}
      <Card className="divide-y divide-hint/20 overflow-hidden">
        <ActionRow icon={<ExternalLink />} onClick={() => openLink(giveaway.post_link)}>
          {t("details.post")}
        </ActionRow>
        {changeable && (
          <>
            <ActionRow icon={<ChevronRight />} onClick={() => props.onEdit(giveaway)}>
              {t("details.edit")}
            </ActionRow>
            <ActionRow onClick={() => setAsking("move")}>{t("details.move")}</ActionRow>
          </>
        )}
        {running && <ActionRow onClick={() => setAsking("finish-now")}>{t("details.finishNow")}</ActionRow>}
      </Card>
      {running && (
        <Card className="overflow-hidden">
          <ActionRow danger onClick={() => setAsking("cancel")}>
            {t("details.cancelGiveaway")}
          </ActionRow>
        </Card>
      )}
      <Confirm
        open={asking === "finish-now"}
        title={t("details.finishTitle")}
        text={t("details.finishText")}
        confirm={t("details.finishNow")}
        busy={busy}
        onConfirm={() => act("finish-now", "details.finishing")}
        onClose={() => setAsking(undefined)}
      />
      <Confirm
        open={asking === "cancel"}
        title={t("details.cancelTitle")}
        text={t("details.cancelText")}
        confirm={t("details.cancelGiveaway")}
        danger
        busy={busy}
        onConfirm={() => act("cancel", "details.cancelled")}
        onClose={() => setAsking(undefined)}
      />
      <Dialog
        open={asking === "move"}
        title={t("details.moveTitle")}
        onClose={() => setAsking(undefined)}
        actions={
          <>
            <Button onClick={() => setAsking(undefined)}>{t("app.cancel")}</Button>
            <Button variant="primary" onClick={move} busy={busy} disabled={!link}>
              {t("details.move")}
            </Button>
          </>
        }
      >
        <Field
          label={t("form.postLink")}
          value={link}
          onChange={(e) => setLink(e.target.value)}
          placeholder="https://t.me/channel/124"
          autoFocus
          error={moveError ? t(`error.${moveError.code}`) : undefined}
          help={t("details.moveHelp")}
          inputMode="url"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
        />
      </Dialog>
    </Section>
  );
}
