import { EyeOff } from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { api, Failure, must, type Giveaway } from "../api/client";
import { useT, type T } from "../i18n";
import { defaultEnd } from "../time";
import { Button, Card, ErrorText, Row, Section, SectionTitle, Title, useNotify } from "../ui";

type Field = "post_link" | "ends_at" | "winners" | "button_text" | "admin_label";

type Values = { ends_at: string; winners: number; button_text: string; admin_label: string };

// changed returns only the fields an edit changes, so an untouched end time that has passed
// does not block a change to another field.
function changed(g: Giveaway, v: Values): Partial<Values> {
  return {
    ...(v.ends_at !== g.ends_at && { ends_at: v.ends_at }),
    ...(v.winners !== g.winners && { winners: v.winners }),
    ...(v.button_text !== g.button_text && { button_text: v.button_text }),
    ...(v.admin_label !== g.admin_label && { admin_label: v.admin_label }),
  };
}

// controlClass styles a control inside a form card, with a red ring when the field is invalid.
function controlClass(invalid: boolean): string {
  return `min-h-12 text-base text-fg focus-visible:outline-2 focus-visible:outline-accent ${invalid ? "ring-2 ring-danger" : ""}`;
}

// boxClass is a compact control in a light filled box, as iOS shows an editable value.
function boxClass(invalid: boolean): string {
  return `${controlClass(invalid)} rounded-lg bg-hint/15 px-2`;
}

// FieldNote is the help or error text under a field's card.
function FieldNote({ id, error, children }: { id: string; error?: boolean; children: ReactNode }) {
  return (
    <p id={`${id}-note`} className={`px-4 text-sm ${error ? "text-danger" : "text-hint"}`}>
      {children}
    </p>
  );
}

// FieldCard is one form field in its own card: a row with the label and the control, and the
// field's help or error text under the card.
function FieldCard(props: { id: string; label: string; htmlFor?: string; wrap?: boolean; note?: ReactNode; error?: boolean; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Card>
        <Row label={props.label} htmlFor={props.htmlFor} wrap={props.wrap}>
          {props.children}
        </Row>
      </Card>
      {props.note && (
        <FieldNote id={props.id} error={props.error}>
          {props.note}
        </FieldNote>
      )}
    </div>
  );
}

// noteProps marks a control invalid and links it to the note under its card, when there is one.
function noteProps(id: string, error?: string, hasNote = true) {
  return { "aria-invalid": error ? true : undefined, "aria-describedby": hasNote || error ? `${id}-note` : undefined };
}

const hours = Array.from({ length: 24 }, (_, i) => String(i).padStart(2, "0"));
const minutes = Array.from({ length: 60 }, (_, i) => String(i).padStart(2, "0"));

// TimeSelect is a native list styled like the other boxes: macOS draws its own select at a
// fixed small height, so the app removes that look and draws the arrow itself.
function TimeSelect(props: {
  label: string;
  value: string;
  options: string[];
  onChange: (v: string) => void;
  error: { "aria-invalid"?: boolean; "aria-describedby"?: string };
}) {
  return (
    <div className="relative w-[4.5rem] shrink-0">
      <select
        aria-label={props.label}
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        className={`${boxClass(!!props.error["aria-invalid"])} w-full appearance-none pr-7`}
        {...props.error}
      >
        {props.options.map((o) => (
          <option key={o}>{o}</option>
        ))}
      </select>
      <svg aria-hidden viewBox="0 0 24 24" className="pointer-events-none absolute top-1/2 right-1.5 size-5 -translate-y-1/2 fill-current text-hint">
        <path d="M7 10l5 5 5-5z" />
      </svg>
    </div>
  );
}

// EndField picks the end as a native date and the time as two lists, so the time is always
// 24-hour: a datetime-local input shows AM/PM on devices set to a 12-hour locale. On a narrow
// phone the controls wrap under the label.
function EndField({ value, onChange, error, t }: { value: string; onChange: (v: string) => void; error?: string; t: T }) {
  const id = useId();
  const [date = "", time = "19:00"] = value.split("T");
  const [hour = "19", minute = "00"] = time.split(":");
  const set = (d: string, h: string, m: string) => onChange(`${d}T${h}:${m}`);
  const props = noteProps(id, error, false);

  return (
    <FieldCard id={id} label={t("form.endsAt")} wrap note={error} error>
      <input
        type="date"
        required
        aria-label={t("form.date")}
        // Uncontrolled: while a part of the date is being edited, Safari's WebView reports "",
        // and a controlled input would write the old date back. Only complete dates count.
        defaultValue={date}
        onChange={(e) => e.target.value && set(e.target.value, hour, minute)}
        className={`${boxClass(!!error)} min-w-0 flex-1`}
        {...props}
      />
      <TimeSelect label={t("form.hour")} value={hour} options={hours} onChange={(h) => set(date, h, minute)} error={props} />
      <span aria-hidden>:</span>
      <TimeSelect label={t("form.minute")} value={minute} options={minutes} onChange={(m) => set(date, hour, m)} error={props} />
    </FieldCard>
  );
}

// GiveawayForm creates a giveaway, or edits one when given. A failed submit keeps every value
// and shows the error next to the field it concerns. onDirty hears whether the values differ
// from the ones the form opened with, and false once the form closes.
export default function GiveawayForm(props: { giveaway?: Giveaway; onDone: (g: Giveaway) => void; onDirty: (dirty: boolean) => void }) {
  const { giveaway, onDone, onDirty } = props;
  const t = useT();
  const notify = useNotify();
  const id = useId();
  const [initial] = useState(() => ({
    post_link: "",
    ends_at: giveaway?.ends_at || defaultEnd(),
    winners: String(giveaway?.winners ?? 1),
    button_text: giveaway?.button_text ?? t("form.defaultButton"),
    admin_label: giveaway?.admin_label ?? "",
  }));
  const [values, setValues] = useState(initial);
  const dirty = (Object.keys(values) as Field[]).some((k) => values[k] !== initial[k]);

  useEffect(() => {
    onDirty(dirty);
    return () => onDirty(false);
  }, [dirty, onDirty]);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<Failure>();
  // open is false once the form has closed, so a save that finishes after the admin left it
  // through "Discard changes?" does not pull them back to the giveaway.
  const open = useRef(true);
  useEffect(() => {
    open.current = true;
    return () => {
      open.current = false;
    };
  }, []);

  const set = (field: Field) => (e: { target: { value: string } }) => setValues({ ...values, [field]: e.target.value });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setFailure(undefined);
    const body = { ends_at: values.ends_at, winners: Number(values.winners), button_text: values.button_text, admin_label: values.admin_label };
    try {
      const g = giveaway
        ? await must(
            api.PATCH("/api/giveaways/{id}", { params: { path: { id: giveaway.id } }, body: changed(giveaway, body) }),
          )
        : await must(api.POST("/api/giveaways", { body: { ...body, post_link: values.post_link } }));
      notify(t(giveaway ? "form.saved" : "form.created"));
      if (open.current) onDone(g);
    } catch (err) {
      setFailure(err instanceof Failure ? err : new Failure(undefined));
    } finally {
      setBusy(false);
    }
  };

  // fieldError is the error text of field, when the last failure concerns it.
  const fieldError = (field: Field) => (failure?.field === field ? t(`error.${failure.code}`) : undefined);
  const general = failure && !failure.field;
  const linkError = fieldError("post_link");
  const winnersError = fieldError("winners");
  const buttonError = fieldError("button_text");

  return (
    <form onSubmit={submit} noValidate>
      <Section>
        <Title>{t(giveaway ? "form.editTitle" : "form.newTitle")}</Title>
        {general && <ErrorText error={failure} />}
        {!giveaway && (
          <div className="flex flex-col gap-1.5">
            <SectionTitle>
              <label htmlFor={`${id}-link`}>{t("form.postLink")}</label>
            </SectionTitle>
            <Card>
              <input
                id={`${id}-link`}
                value={values.post_link}
                onChange={set("post_link")}
                placeholder="https://t.me/channel/123"
                required
                autoFocus
                inputMode="url"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                {...noteProps(`${id}-link`, linkError)}
                className={`${controlClass(!!linkError)} w-full rounded-2xl bg-transparent px-4`}
              />
            </Card>
            <FieldNote id={`${id}-link`} error={!!linkError}>
              {linkError ?? t("form.postLinkHelp")}
            </FieldNote>
          </div>
        )}
        <FieldCard
          id={`${id}-label`}
          label={t("form.adminLabel")}
          htmlFor={`${id}-label`}
          note={
            <>
              <EyeOff aria-hidden className="mr-1 inline size-4 align-[-3px]" />
              {t("form.adminLabelNote")}
            </>
          }
        >
          <input
            id={`${id}-label`}
            value={values.admin_label}
            onChange={set("admin_label")}
            maxLength={64}
            placeholder={t("form.adminLabelPlaceholder")}
            {...noteProps(`${id}-label`)}
            className={`${controlClass(false)} w-full min-w-0 rounded-lg bg-transparent text-right`}
          />
        </FieldCard>
        <EndField
          value={values.ends_at}
          onChange={(ends_at) => setValues({ ...values, ends_at })}
          error={fieldError("ends_at")}
          t={t}
        />
        <FieldCard id={`${id}-winners`} label={t("form.winners")} htmlFor={`${id}-winners`} note={winnersError ?? t("form.winnersHelp")} error={!!winnersError}>
          <input
            id={`${id}-winners`}
            type="number"
            value={values.winners}
            onChange={set("winners")}
            required
            min={1}
            max={98}
            step={1}
            inputMode="numeric"
            {...noteProps(`${id}-winners`, winnersError)}
            className={`${boxClass(!!winnersError)} w-16 [appearance:textfield] text-center [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none`}
          />
        </FieldCard>
        <FieldCard id={`${id}-button`} label={t("form.buttonText")} htmlFor={`${id}-button`} note={buttonError ?? t("form.buttonTextHelp")} error={!!buttonError}>
          <input
            id={`${id}-button`}
            value={values.button_text}
            onChange={set("button_text")}
            required
            maxLength={64}
            {...noteProps(`${id}-button`, buttonError)}
            className={`${controlClass(!!buttonError)} w-full min-w-0 rounded-lg bg-transparent text-right`}
          />
        </FieldCard>
        <Button type="submit" variant="primary" full busy={busy}>
          {t(giveaway ? "app.save" : "form.create")}
        </Button>
      </Section>
    </form>
  );
}
