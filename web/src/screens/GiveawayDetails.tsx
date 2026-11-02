import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { useState } from "react";
import { api, Failure, must, type Giveaway } from "../api/client";
import { useT, type TextKey } from "../i18n";
import { openLink } from "../telegram";
import { formatEnd } from "../time";
import { Confirm, ErrorText, Loading, Section, statusColor, useLoad, useNotify } from "../ui";

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
  const giveaway = g.data;

  // run calls an action and shows its result; a stale giveaway reloads (someone changed it).
  const run = async (call: () => Promise<Giveaway>, done: TextKey) => {
    setBusy(true);
    setFailure(undefined);
    try {
      g.set(await call());
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
      <Stack direction="row" spacing={1} sx={{ alignItems: "center", justifyContent: "space-between" }}>
        <Typography variant="h6" component="h1" noWrap>
          {giveaway.button_text}
        </Typography>
        <Chip label={t(`status.${giveaway.status}`)} color={statusColor(giveaway.status)} />
      </Stack>
      {failure !== undefined && !moveError && <ErrorText error={failure} />}
      <Paper sx={{ p: 2 }}>
        <Typography>
          {giveaway.ends_at ? t("list.ends", { time: formatEnd(giveaway.ends_at) }) : t("list.noEnd")}
        </Typography>
        <Typography color="text.secondary">{t("list.participants", { n: giveaway.participants })}</Typography>
        <Typography color="text.secondary">
          {t("form.winners")}: {giveaway.winners}
        </Typography>
      </Paper>
      <Button variant="outlined" size="large" fullWidth onClick={() => openLink(giveaway.post_link)}>
        {t("details.post")}
      </Button>
      {giveaway.status === "review" && (
        <Button variant="contained" size="large" fullWidth onClick={props.onReview}>
          {t("details.openReview")}
        </Button>
      )}
      {changeable && (
        <>
          <Button variant="outlined" size="large" fullWidth onClick={() => props.onEdit(giveaway)}>
            {t("details.edit")}
          </Button>
          <Button variant="outlined" size="large" fullWidth onClick={() => setAsking("move")}>
            {t("details.move")}
          </Button>
        </>
      )}
      {running && (
        <>
          <Button variant="outlined" size="large" fullWidth onClick={() => setAsking("finish-now")}>
            {t("details.finishNow")}
          </Button>
          <Button color="error" size="large" fullWidth onClick={() => setAsking("cancel")}>
            {t("details.cancelGiveaway")}
          </Button>
        </>
      )}
      {giveaway.status === "cancelled" && (
        <Button
          variant="contained"
          size="large"
          fullWidth
          loading={busy}
          onClick={() => act("restore", "details.restored")}
        >
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
      <Dialog open={asking === "move"} onClose={() => setAsking(undefined)} fullWidth maxWidth="xs" aria-labelledby="move-title">
        <DialogTitle id="move-title">{t("details.moveTitle")}</DialogTitle>
        <DialogContent>
          <TextField
            label={t("form.postLink")}
            value={link}
            onChange={(e) => setLink(e.target.value)}
            placeholder="https://t.me/channel/124"
            autoFocus
            fullWidth
            margin="dense"
            error={!!moveError}
            helperText={moveError ? t(`error.${moveError.code}`) : t("details.moveHelp")}
            slotProps={{ htmlInput: { inputMode: "url", autoCapitalize: "none", autoCorrect: "off", spellCheck: false } }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAsking(undefined)}>{t("app.cancel")}</Button>
          <Button variant="contained" onClick={move} loading={busy} disabled={!link}>
            {t("details.move")}
          </Button>
        </DialogActions>
      </Dialog>
    </Section>
  );
}
