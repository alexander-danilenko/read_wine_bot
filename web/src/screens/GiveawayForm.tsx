import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { useState, type FormEvent } from "react";
import { api, Failure, must, type Giveaway } from "../api/client";
import { useT } from "../i18n";
import { defaultEnd } from "../time";
import { ErrorText, Section, useNotify } from "../ui";

type Field = "post_link" | "ends_at" | "winners" | "button_text";

type Values = { ends_at: string; winners: number; button_text: string };

// changed returns only the fields an edit changes, so an untouched end time that has passed
// does not block a change to another field.
function changed(g: Giveaway, v: Values): Partial<Values> {
  return {
    ...(v.ends_at !== g.ends_at && { ends_at: v.ends_at }),
    ...(v.winners !== g.winners && { winners: v.winners }),
    ...(v.button_text !== g.button_text && { button_text: v.button_text }),
  };
}

// GiveawayForm creates a giveaway, or edits one when given. A failed submit keeps every value
// and shows the error next to the field it concerns.
export default function GiveawayForm({ giveaway, onDone }: { giveaway?: Giveaway; onDone: (g: Giveaway) => void }) {
  const t = useT();
  const notify = useNotify();
  const [values, setValues] = useState({
    post_link: "",
    ends_at: giveaway?.ends_at || defaultEnd(),
    winners: String(giveaway?.winners ?? 1),
    button_text: giveaway?.button_text ?? t("form.defaultButton"),
  });
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<Failure>();

  const set = (field: Field) => (e: { target: { value: string } }) => setValues({ ...values, [field]: e.target.value });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setFailure(undefined);
    const body = { ends_at: values.ends_at, winners: Number(values.winners), button_text: values.button_text };
    try {
      const g = giveaway
        ? await must(
            api.PATCH("/api/giveaways/{id}", { params: { path: { id: giveaway.id } }, body: changed(giveaway, body) }),
          )
        : await must(api.POST("/api/giveaways", { body: { ...body, post_link: values.post_link } }));
      notify(t(giveaway ? "form.saved" : "form.created"));
      onDone(g);
    } catch (err) {
      setFailure(err instanceof Failure ? err : new Failure(undefined));
    } finally {
      setBusy(false);
    }
  };

  // fieldError is the error text of field, when the last failure concerns it.
  const fieldError = (field: Field) => (failure?.field === field ? t(`error.${failure.code}`) : undefined);
  const general = failure && !failure.field;

  return (
    <form onSubmit={submit} noValidate>
      <Section>
        <Typography variant="h6" component="h1">
          {t(giveaway ? "form.editTitle" : "form.newTitle")}
        </Typography>
        {general && <ErrorText error={failure} />}
        {!giveaway && (
          <TextField
            label={t("form.postLink")}
            value={values.post_link}
            onChange={set("post_link")}
            placeholder="https://t.me/channel/123"
            required
            autoFocus
            fullWidth
            error={!!fieldError("post_link")}
            helperText={fieldError("post_link") ?? t("form.postLinkHelp")}
            slotProps={{ htmlInput: { inputMode: "url", autoCapitalize: "none", autoCorrect: "off", spellCheck: false } }}
          />
        )}
        <TextField
          label={t("form.endsAt")}
          type="datetime-local"
          value={values.ends_at}
          onChange={set("ends_at")}
          required
          fullWidth
          error={!!fieldError("ends_at")}
          helperText={fieldError("ends_at")}
          slotProps={{ inputLabel: { shrink: true } }}
        />
        <TextField
          label={t("form.winners")}
          type="number"
          value={values.winners}
          onChange={set("winners")}
          required
          fullWidth
          error={!!fieldError("winners")}
          helperText={fieldError("winners") ?? t("form.winnersHelp")}
          slotProps={{ htmlInput: { min: 1, max: 98, step: 1, inputMode: "numeric" } }}
        />
        <TextField
          label={t("form.buttonText")}
          value={values.button_text}
          onChange={set("button_text")}
          required
          fullWidth
          error={!!fieldError("button_text")}
          helperText={fieldError("button_text") ?? t("form.buttonTextHelp")}
          slotProps={{ htmlInput: { maxLength: 64 } }}
        />
        <Button type="submit" variant="contained" size="large" fullWidth loading={busy}>
          {t(giveaway ? "app.save" : "form.create")}
        </Button>
      </Section>
    </form>
  );
}
