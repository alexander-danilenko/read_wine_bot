import { Languages, Megaphone, Plus } from "lucide-react";
import { useState } from "react";
import { api, must, type Me } from "../api/client";
import { useT, type Lang } from "../i18n";
import { openLink } from "../telegram";
import { Avatar, Button, ErrorText, Loading, Section, Title, useLoad } from "../ui";
import { HelpCard } from "./Help";

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
    <HelpCard icon={<Languages className="size-5" />} title={t("settings.language")}>
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
    </HelpCard>
  );
}

// Channels lists, read-only, the channels where both the bot and the user are admins.
function Channels() {
  const t = useT();
  const list = useLoad(() => must(api.GET("/api/channels")), []);

  let body;
  if (list.loading && !list.data) body = <Loading />;
  else if (list.error || !list.data) body = <ErrorText error={list.error} onRetry={list.reload} />;
  else if (list.data.channels.length === 0) body = <p className="text-hint">{t("settings.noChannels")}</p>;
  else {
    body = (
      <ul className="-mx-4 divide-y divide-hint/20">
        {list.data.channels.map((c) => (
          <li key={c.id} className="flex min-h-14 items-center gap-3 px-4 py-2">
            <Avatar channel={c} />
            <span className="min-w-0 flex-1 truncate">{c.title}</span>
          </li>
        ))}
      </ul>
    );
  }

  return (
    <HelpCard icon={<Megaphone className="size-5" />} title={t("settings.channels")}>
      <p className="-mt-2 text-sm text-hint">{t("settings.channelsHint")}</p>
      {body}
    </HelpCard>
  );
}

// Settings shows the language picker, the user's channels and the link that adds the bot to
// another channel with the rights giveaways need.
export default function Settings({ me, onMe }: { me: Me; onMe: (me: Me) => void }) {
  const t = useT();
  return (
    <Section>
      <Title>{t("settings.title")}</Title>
      <LanguagePicker me={me} onMe={onMe} />
      <Channels />
      <Button variant="primary" full onClick={() => openLink(me.add_link)}>
        <Plus aria-hidden className="size-5" />
        {t("settings.add")}
      </Button>
    </Section>
  );
}
