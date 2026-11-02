import { useState } from "react";
import { api, Failure, must, type Giveaway } from "../api/client";
import { useT, type TextKey } from "../i18n";
import { openLink } from "../telegram";
import { formatEnd } from "../time";
import { Avatar, Button, Card, Confirm, Dialog, ErrorText, Field, Loading, Pill, Section, statusTone, Title, useLoad, useNotify } from "../ui";

type Action = "cancel" | "restore" | "finish-now";

export default function GiveawayDetails(props: { id: number; onEdit: (g: Giveaway) => void; onReview: () => void }) {
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

  const running = giveaway.status === "active";
  const changeable = running || giveaway.status === "cancelled";
  const moveError = asking === "move" && failure instanceof Failure && failure.field ? failure : undefined;

  return (
    <Section>
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-3">
          <Avatar channel={channel} size="size-12" />
          <Title>{channel.title || giveaway.post_link}</Title>
        </div>
        <Pill tone={statusTone(giveaway.status)}>{t(`status.${giveaway.status}`)}</Pill>
      </div>
      {failure !== undefined && !moveError && <ErrorText error={failure} />}
      <Card className="flex flex-col gap-1 p-4">
        <p>{giveaway.ends_at ? t("list.ends", { time: formatEnd(giveaway.ends_at) }) : t("list.noEnd")}</p>
        <p className="text-hint">{t("list.participants", { n: giveaway.participants })}</p>
        <p className="text-hint">
          {t("form.winners")}: {giveaway.winners}
        </p>
        <p className="text-hint">
          {t("form.buttonText")}: {giveaway.button_text}
        </p>
      </Card>
      <Button variant="outline" full onClick={() => openLink(giveaway.post_link)}>
        {t("details.post")}
      </Button>
      {giveaway.status === "review" && (
        <Button variant="primary" full onClick={props.onReview}>
          {t("details.openReview")}
        </Button>
      )}
      {changeable && (
        <>
          <Button variant="outline" full onClick={() => props.onEdit(giveaway)}>
            {t("details.edit")}
          </Button>
          <Button variant="outline" full onClick={() => setAsking("move")}>
            {t("details.move")}
          </Button>
        </>
      )}
      {running && (
        <>
          <Button variant="outline" full onClick={() => setAsking("finish-now")}>
            {t("details.finishNow")}
          </Button>
          <Button variant="danger-text" full onClick={() => setAsking("cancel")}>
            {t("details.cancelGiveaway")}
          </Button>
        </>
      )}
      {giveaway.status === "cancelled" && (
        <Button variant="primary" full busy={busy} onClick={() => act("restore", "details.restored")}>
          {t("details.restore")}
        </Button>
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
