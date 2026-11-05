import { ChevronRight, Handshake, Languages, Megaphone, Plus } from "lucide-react";
import { useState, type ReactNode } from "react";
import { api, must, type Channel, type ChannelLists, type Me } from "../api/client";
import { useT, type Lang } from "../i18n";
import { openLink } from "../telegram";
import { Avatar, Button, ErrorText, Loading, Section, Title, TitledCard, useLoad } from "../ui";

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

// ChannelRow is one channel: a link to it when it has a public username, plain text otherwise.
function ChannelRow({ channel }: { channel: Channel }) {
  const content = (
    <>
      <Avatar channel={channel} />
      <span className="min-w-0 flex-1 truncate">{channel.title}</span>
    </>
  );
  if (!channel.username) return <div className="flex min-h-14 items-center gap-3 px-4 py-2">{content}</div>;
  return (
    <button
      type="button"
      onClick={() => openLink(`https://t.me/${channel.username}`)}
      className="flex min-h-14 w-full items-center gap-3 px-4 py-2 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent"
    >
      {content}
      <ChevronRight aria-hidden className="size-5 shrink-0 text-hint" />
    </button>
  );
}

// ChannelCard lists channels read-only, or shows loading, a retryable error or the empty text.
function ChannelCard({
  icon,
  title,
  hint,
  empty,
  list,
  pick,
}: {
  icon: ReactNode;
  title: string;
  hint: string;
  empty: string;
  list: ReturnType<typeof useLoad<ChannelLists>>;
  pick: (lists: ChannelLists) => Channel[];
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
      <p className="-mt-2 text-sm text-hint">{hint}</p>
      {body}
    </TitledCard>
  );
}

// Settings shows the language picker, the user's channels, the partner channels (every other
// channel where the bot is an admin, shown to everyone) and the link that adds the bot to
// another channel with the rights giveaways need.
export default function Settings({ me, onMe }: { me: Me; onMe: (me: Me) => void }) {
  const t = useT();
  const channels = useLoad(() => must(api.GET("/api/channels")), []);
  return (
    <Section>
      <Title>{t("settings.title")}</Title>
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
        hint={t("settings.partnersHint")}
        empty={t("settings.noPartners")}
        list={channels}
        pick={(l) => l.partners}
      />
      <Button variant="primary" full onClick={() => openLink(me.add_link)}>
        <Plus aria-hidden className="size-5" />
        {t("settings.add")}
      </Button>
    </Section>
  );
}
