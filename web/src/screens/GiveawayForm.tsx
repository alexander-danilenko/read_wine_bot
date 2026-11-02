import { useId, useState, type FormEvent } from "react";
import { api, Failure, must, type Giveaway } from "../api/client";
import { useT, type T } from "../i18n";
import { defaultEnd } from "../time";
import { Button, ErrorText, Field, inputClass, Section, Title, useNotify } from "../ui";

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

const hours = Array.from({ length: 24 }, (_, i) => String(i).padStart(2, "0"));
const minutes = Array.from({ length: 60 }, (_, i) => String(i).padStart(2, "0"));

// TimeSelect is a native list styled like the other inputs: macOS draws its own select at a
// fixed small height, so the app removes that look and draws the arrow itself.
function TimeSelect(props: {
  label: string;
  value: string;
  options: string[];
  onChange: (v: string) => void;
  error: { "aria-invalid"?: boolean; "aria-describedby"?: string };
}) {
  const invalid = !!props.error["aria-invalid"];
  return (
    <div className="relative w-24 shrink-0">
      <select
        aria-label={props.label}
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        className={`${inputClass(invalid)} w-full appearance-none pr-8`}
        {...props.error}
      >
        {props.options.map((o) => (
          <option key={o}>{o}</option>
        ))}
      </select>
      <svg aria-hidden viewBox="0 0 24 24" className="pointer-events-none absolute top-1/2 right-2 size-5 -translate-y-1/2 fill-current text-hint">
        <path d="M7 10l5 5 5-5z" />
      </svg>
    </div>
  );
}

// EndField picks the end as a native date and the time as two lists, so the time is always
// 24-hour: a datetime-local input shows AM/PM on devices set to a 12-hour locale.
function EndField({ value, onChange, error, t }: { value: string; onChange: (v: string) => void; error?: string; t: T }) {
  const id = useId();
  const [date = "", time = "19:00"] = value.split("T");
  const [hour = "19", minute = "00"] = time.split(":");
  const set = (d: string, h: string, m: string) => onChange(`${d}T${h}:${m}`);
  const props = { "aria-invalid": error ? true : undefined, "aria-describedby": error ? `${id}-note` : undefined };

  return (
    <fieldset className="flex flex-col gap-1">
      <legend className="mb-1 text-sm font-medium">
        {t("form.endsAt")}
        <span aria-hidden> *</span>
      </legend>
      <div className="flex items-center gap-2">
        <input
          type="date"
          required
          aria-label={t("form.date")}
          // Uncontrolled: while a part of the date is being edited, Safari's WebView reports "",
          // and a controlled input would write the old date back. Only complete dates count.
          defaultValue={date}
          onChange={(e) => e.target.value && set(e.target.value, hour, minute)}
          className={`${inputClass(!!error)} min-w-0 flex-1`}
          {...props}
        />
        <TimeSelect label={t("form.hour")} value={hour} options={hours} onChange={(h) => set(date, h, minute)} error={props} />
        <span aria-hidden>:</span>
        <TimeSelect label={t("form.minute")} value={minute} options={minutes} onChange={(m) => set(date, hour, m)} error={props} />
      </div>
      {error && (
        <p id={`${id}-note`} className="text-sm text-danger">
          {error}
        </p>
      )}
    </fieldset>
  );
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
        <Title>{t(giveaway ? "form.editTitle" : "form.newTitle")}</Title>
        {general && <ErrorText error={failure} />}
        {!giveaway && (
          <Field
            label={t("form.postLink")}
            value={values.post_link}
            onChange={set("post_link")}
            placeholder="https://t.me/channel/123"
            required
            autoFocus
            error={fieldError("post_link")}
            help={t("form.postLinkHelp")}
            inputMode="url"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
          />
        )}
        <EndField
          value={values.ends_at}
          onChange={(ends_at) => setValues({ ...values, ends_at })}
          error={fieldError("ends_at")}
          t={t}
        />
        <Field
          label={t("form.winners")}
          type="number"
          value={values.winners}
          onChange={set("winners")}
          required
          error={fieldError("winners")}
          help={t("form.winnersHelp")}
          min={1}
          max={98}
          step={1}
          inputMode="numeric"
        />
        <Field
          label={t("form.buttonText")}
          value={values.button_text}
          onChange={set("button_text")}
          required
          error={fieldError("button_text")}
          help={t("form.buttonTextHelp")}
          maxLength={64}
        />
        <Button type="submit" variant="primary" full busy={busy}>
          {t(giveaway ? "app.save" : "form.create")}
        </Button>
      </Section>
    </form>
  );
}
