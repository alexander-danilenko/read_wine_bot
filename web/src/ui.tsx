import { Copy } from "lucide-react";
import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import { Failure, type Channel, type Giveaway } from "./api/client";
import { useT } from "./i18n";

const focus = "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

function Spinner({ className = "size-5" }: { className?: string }) {
  return <span aria-hidden className={`${className} animate-spin rounded-full border-2 border-current border-t-transparent`} />;
}

export function Loading() {
  const t = useT();
  return (
    <div role="status" className="flex justify-center py-12 text-accent">
      <Spinner className="size-8" />
      <span className="sr-only">{t("app.loading")}</span>
    </div>
  );
}

const variants = {
  primary: "bg-accent text-on-accent",
  danger: "bg-danger text-on-danger",
  outline: "border border-accent text-accent",
  text: "text-accent",
  "danger-text": "text-danger",
};

// Button is at least 48 px high, the touch target size Material and Apple recommend.
export function Button({
  variant = "text",
  busy,
  full,
  className = "",
  children,
  disabled,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: keyof typeof variants; busy?: boolean; full?: boolean }) {
  return (
    <button
      type="button"
      {...rest}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={`inline-flex min-h-12 items-center justify-center gap-2 rounded-xl px-4 font-medium disabled:opacity-50 ${focus} ${variants[variant]} ${full ? "w-full" : ""} ${className}`}
    >
      {busy && <Spinner />}
      {children}
    </button>
  );
}

// pagerSlots lists what Pager shows between its arrows. Every page is shown up to four pages;
// past that, the first, the last and the current page, with "gap" for the pages left out. With
// 48 px buttons and narrow gaps the bar stays within 328 px, a 360 px phone less its margins.
export function pagerSlots(page: number, pages: number): (number | "gap")[] {
  if (pages <= 4) return Array.from({ length: pages }, (_, i) => i + 1);
  if (page <= 3) return [1, 2, 3, "gap", pages];
  if (page >= pages - 2) return [1, "gap", pages - 2, pages - 1, pages];
  return [1, "gap", page, "gap", pages];
}

// Pager moves between the pages of a list. It shows nothing for a single page.
export function Pager({ page, pages, onPage }: { page: number; pages: number; onPage: (page: number) => void }) {
  const t = useT();
  if (pages <= 1) return null;

  const cell = `inline-flex size-12 items-center justify-center rounded-xl ${focus}`;
  const arrow = (to: number, label: string, glyph: string) => (
    <li>
      <button type="button" className={`${cell} text-accent disabled:opacity-30`} disabled={to < 1 || to > pages} aria-label={label} onClick={() => onPage(to)}>
        <span aria-hidden>{glyph}</span>
      </button>
    </li>
  );

  return (
    <nav aria-label={t("list.pages")}>
      <ul className="flex items-center justify-center">
        {arrow(page - 1, t("list.prev"), "‹")}
        {pagerSlots(page, pages).map((slot, i) =>
          slot === "gap" ? (
            <li key={`gap-${i}`} aria-hidden className="inline-flex h-12 w-6 items-center justify-center text-hint">
              …
            </li>
          ) : (
            <li key={slot}>
              <button
                type="button"
                className={`${cell} ${slot === page ? "bg-accent font-medium text-on-accent" : "text-accent"}`}
                aria-current={slot === page ? "page" : undefined}
                aria-label={t("list.page", { n: slot })}
                onClick={() => onPage(slot)}
              >
                {slot}
              </button>
            </li>
          ),
        )}
        {arrow(page + 1, t("list.next"), "›")}
      </ul>
    </nav>
  );
}

// ErrorText shows the text for a failed call, with a retry when given.
export function ErrorText({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const t = useT();
  const code = error instanceof Failure ? error.code : "internal";
  return (
    <div role="alert" className="flex items-center gap-3 rounded-xl border border-danger px-4 py-2 text-danger">
      <span className="flex-1 py-2">{t(`error.${code}`)}</span>
      {onRetry && (
        <Button variant="danger-text" onClick={onRetry}>
          {t("app.retry")}
        </Button>
      )}
    </div>
  );
}

// inputClass styles a text input or select; the hint-colored border passes 3:1 (WCAG 1.4.11).
export function inputClass(error: boolean): string {
  return `min-h-12 rounded-xl border bg-card px-3 text-base text-fg ${focus} ${error ? "border-danger" : "border-hint"}`;
}

// Field is a labelled text input with its help or error text under it.
export function Field({
  label,
  help,
  error,
  ...input
}: InputHTMLAttributes<HTMLInputElement> & { label: string; help?: string; error?: string }) {
  const id = useId();
  const note = error ?? help;
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
        {input.required && <span aria-hidden> *</span>}
      </label>
      <input
        id={id}
        {...input}
        aria-invalid={error ? true : undefined}
        aria-describedby={note ? `${id}-note` : undefined}
        className={`${inputClass(!!error)} w-full`}
      />
      {note && (
        <p id={`${id}-note`} className={`text-sm ${error ? "text-danger" : "text-hint"}`}>
          {note}
        </p>
      )}
    </div>
  );
}

// Dialog is a modal on the native <dialog>, which traps focus and closes on Escape.
export function Dialog(props: { open: boolean; title: string; onClose: () => void; children: ReactNode; actions: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const d = ref.current;
    if (props.open && !d?.open) d?.showModal();
    if (!props.open && d?.open) d.close();
  }, [props.open]);

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onClose={props.onClose}
      onClick={(e) => e.target === ref.current && props.onClose()} // a tap on the backdrop
      className="m-auto w-[calc(100%-2rem)] max-w-sm rounded-2xl bg-card p-0 text-fg backdrop:bg-black/50"
    >
      <div className="flex flex-col gap-4 p-5">
        <h2 id={titleId} className="text-lg font-semibold">
          {props.title}
        </h2>
        {props.children}
        <div className="flex justify-end gap-2">{props.actions}</div>
      </div>
    </dialog>
  );
}

// Confirm asks before an action that cannot be undone (docs/telegram-ux.md).
export function Confirm(props: {
  open: boolean;
  title: string;
  text: string;
  confirm: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const t = useT();
  return (
    <Dialog
      open={props.open}
      title={props.title}
      onClose={props.onClose}
      actions={
        <>
          <Button onClick={props.onClose}>{t("app.cancel")}</Button>
          <Button variant={props.danger ? "danger" : "primary"} busy={props.busy} onClick={props.onConfirm}>
            {props.confirm}
          </Button>
        </>
      }
    >
      <p className="text-hint">{props.text}</p>
    </Dialog>
  );
}

const tones = { ok: "text-ok", warn: "text-warn", info: "text-accent", neutral: "text-hint", danger: "text-danger" };

// Pill is a short outlined label, such as a status. Its text says it all; color only helps.
export function Pill({ tone, children }: { tone: keyof typeof tones; children: ReactNode }) {
  return <span className={`shrink-0 rounded-full border border-current px-2.5 py-0.5 text-sm ${tones[tone]}`}>{children}</span>;
}

// statusTone is the pill tone of a giveaway status.
export function statusTone(status: Giveaway["status"]): keyof typeof tones {
  switch (status) {
    case "active":
      return "ok";
    case "review":
      return "warn";
    case "cancelled":
      return "neutral";
    default:
      return "info";
  }
}

// Avatar is the channel photo in a circle, or the title's first letter when there is no photo.
// It is decorative: the title next to it names the channel.
export function Avatar({ channel, size = "size-10" }: { channel?: Channel; size?: string }) {
  const [broken, setBroken] = useState(false);
  if (channel?.photo && !broken) {
    return (
      <img
        src={channel.photo}
        alt=""
        loading="lazy"
        onError={() => setBroken(true)}
        className={`${size} shrink-0 rounded-full object-cover`}
      />
    );
  }
  return (
    <span aria-hidden className={`${size} flex shrink-0 items-center justify-center rounded-full bg-accent font-medium text-on-accent`}>
      {[...(channel?.title ?? "")][0] ?? "#"}
    </span>
  );
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`rounded-2xl bg-card ${className}`}>{children}</div>;
}

// TitledCard is a card under a heading, with an icon that marks what the card is about.
export function TitledCard({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
  return (
    <Card className="flex flex-col gap-4 p-4">
      <h2 className="flex items-center gap-3 text-lg font-semibold">
        <span aria-hidden className="flex size-10 shrink-0 items-center justify-center rounded-full bg-accent/15 text-accent">
          {icon}
        </span>
        {title}
      </h2>
      {children}
    </Card>
  );
}

export function Section({ children }: { children: ReactNode }) {
  return <div className="flex flex-col gap-4">{children}</div>;
}

export function Title({ children }: { children: ReactNode }) {
  return <h1 className="truncate text-xl font-semibold">{children}</h1>;
}

// useLoad runs load and keeps its state; reload runs it again.
export function useLoad<D>(load: () => Promise<D>, deps: unknown[]) {
  const [state, setState] = useState<{ data?: D; error?: unknown; loading: boolean }>({ loading: true });
  const [round, setRound] = useState(0);

  useEffect(() => {
    let live = true;
    setState((s) => ({ ...s, loading: true }));
    load().then(
      (data) => live && setState({ data, loading: false }),
      (error: unknown) => live && setState({ error, loading: false }),
    );
    return () => {
      live = false;
    };
  }, [...deps, round]);

  return { ...state, reload: () => setRound((r) => r + 1), set: (data: D) => setState({ data, loading: false }) };
}

// Notify shows a short message at the bottom of the screen.
export const NotifyContext = createContext<(message: string) => void>(() => {});

export function useNotify() {
  return useContext(NotifyContext);
}

// Toast shows message for 4 seconds. The live region stays in the page, so screen readers
// announce each new message.
export function Toast({ message, onDone }: { message?: string; onDone: () => void }) {
  useEffect(() => {
    if (message === undefined) return;
    const timer = setTimeout(onDone, 4000);
    return () => clearTimeout(timer);
  }, [message, onDone]);

  return (
    <div role="status" className="pointer-events-none fixed inset-x-4 bottom-4 flex justify-center">
      {message !== undefined && <div className="max-w-sm rounded-xl bg-fg px-4 py-3 text-page shadow-lg">{message}</div>}
    </div>
  );
}

// PartnerLink copies the partner link. Where the browser refuses the copy, the link shows in a
// read-only field for the user to copy by hand.
export function PartnerLink({ link }: { link: string }) {
  const t = useT();
  const notify = useNotify();
  const [manual, setManual] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(link);
      notify(t("help.copied"));
    } catch {
      setManual(true);
    }
  };

  return (
    <div className="flex flex-col gap-3">
      <Button variant="outline" full onClick={copy}>
        <Copy aria-hidden className="size-5" />
        {t("help.copy")}
      </Button>
      {manual && (
        <label className="flex flex-col gap-1 text-sm">
          {t("help.copyFailed")}
          <input
            readOnly
            value={link}
            aria-label={t("help.copyLabel")}
            onFocus={(e) => e.currentTarget.select()}
            className={`${inputClass(false)} w-full`}
          />
        </label>
      )}
    </div>
  );
}
