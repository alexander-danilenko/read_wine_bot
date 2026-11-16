import { Eye, Handshake, Languages, Megaphone } from "lucide-react";
import { useState, type ReactNode } from "react";
import { api, must, type Channel, type ChannelLists, type Me } from "../api/client";
import { useT, type Lang } from "../i18n";
import { Button, ChannelRow, ErrorText, Loading, PartnerLink, Section, TitledCard, useLoad } from "../ui";

// Each language is shown in its own name, so every user finds theirs.
const languages: { code: Lang; label: string }[] = [
  { code: "en", label: "🇬🇧 English" },
  { code: "uk", label: "🇺🇦 Українська" },
];

// LanguagePicker saves the language the bot and the app use, and hands the answer, the caller
// in the new language, to onMe, so the app redraws without loading again.
function LanguagePicker({ me, onMe }: { me: Me; onMe: (me: Me) => void }) {
  const t = useT();
  const [saving, setSaving] = useState<Lang>();
  const [error, setError] = useState<unknown>();

  const pick = async (code: Lang) => {
    if (code === me.language || saving) return;
    setSaving(code);
    setError(undefined);
    try {
      onMe(await must(api.PUT("/api/me/language", { body: { language: code } })));
    } catch (e) {
      setError(e);
    } finally {
      setSaving(undefined);
    }
  };

  return (
    <TitledCard icon={<Languages className="size-5" />} title={t("settings.language")}>
      <div role="radiogroup" aria-label={t("settings.language")} className="grid grid-cols-2 gap-2">
        {languages.map(({ code, label }) => {
          const selected = me.language === code;
          return (
            <Button
              key={code}
              role="radio"
              aria-checked={selected}
              variant={selected ? "primary" : "outline"}
              busy={saving === code}
              disabled={saving !== undefined && saving !== code}
              onClick={() => pick(code)}
            >
              {label}
            </Button>
          );
        })}
      </div>
      {error !== undefined && <ErrorText error={error} />}
    </TitledCard>
  );
}

// ChannelCard lists channels read-only, or shows loading, a retryable error or the empty text,
// then the card's action.
function ChannelCard({
  icon,
  title,
  hint,
  empty,
  list,
  pick,
  action,
}: {
  icon: ReactNode;
  title: string;
  hint?: string;
  empty: string;
  list: ReturnType<typeof useLoad<ChannelLists>>;
  pick: (lists: ChannelLists) => Channel[];
  action?: ReactNode;
}) {
  let body;
  if (list.loading && !list.data) body = <Loading />;
  else if (list.error || !list.data) body = <ErrorText error={list.error} onRetry={list.reload} />;
  else if (pick(list.data).length === 0) body = <p className="text-hint">{empty}</p>;
  else {
    body = (
      <ul className="-mx-4 divide-y divide-hint/20">
        {pick(list.data).map((c) => (
          <li key={c.id}>
            <ChannelRow channel={c} />
          </li>
        ))}
      </ul>
    );
  }

  return (
    <TitledCard icon={icon} title={title}>
      {hint && <p className="-mt-2 text-sm text-hint">{hint}</p>}
      {body}
      {action}
    </TitledCard>
  );
}

// Settings shows the language picker, the primary channel, and the partner channels with the
// partner link to copy and a way to see the partner screen.
export default function Settings({ me, onMe, onPartner }: { me: Me; onMe: (me: Me) => void; onPartner: () => void }) {
  const t = useT();
  const channels = useLoad(() => must(api.GET("/api/channels")), []);
  return (
    <Section>
      <h1 className="truncate text-3xl font-bold">{t("settings.title")}</h1>
      <LanguagePicker me={me} onMe={onMe} />
      <ChannelCard
        icon={<Megaphone className="size-5" />}
        title={t("settings.channels")}
        hint={t("settings.channelsHint")}
        empty={t("settings.noChannels")}
        list={channels}
        pick={(l) => l.channels}
      />
      <ChannelCard
        icon={<Handshake className="size-5" />}
        title={t("settings.partners")}
        empty={t("settings.noPartners")}
        list={channels}
        pick={(l) => l.partners}
        action={
          <>
            <PartnerLink link={me.partner_link} />
            <Button variant="text" full onClick={onPartner}>
              <Eye aria-hidden className="size-5" />
              {t("settings.partnerPreview")}
            </Button>
          </>
        }
      />
    </Section>
  );
}
