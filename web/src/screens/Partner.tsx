import { CircleX, Handshake, RefreshCw, TriangleAlert, UserPlus } from "lucide-react";
import type { ReactNode } from "react";
import { api, must, type Channel, type Me } from "../api/client";
import { useT, type T, type TextKey } from "../i18n";
import en from "../locales/en.json";
import { openLink } from "../telegram";
import { Button, Card, ChannelRow, ErrorText, Loading, Section, Title, TitledCard, useLoad } from "../ui";

// rightLabel is Telegram's own name of an admin right, or the API's name for a right this app
// does not know yet.
function rightLabel(t: T, right: string): string {
  const key = `right.${right}`;
  return key in en ? t(key as TextKey) : right;
}

function Warning({ children }: { children: ReactNode }) {
  return (
    <div className="flex gap-2 px-4 pb-3 text-sm text-warn">
      <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
      <div>{children}</div>
    </div>
  );
}

// RightsWarning asks the partner to remove the rights the bot has beyond Manage chat. Null means
// the bot could not read its rights there.
function RightsWarning({ channel }: { channel: Channel }) {
  const t = useT();
  if (channel.extra_rights === null) return <Warning>{t("partner.rightsUnknown")}</Warning>;
  if (channel.extra_rights.length === 0) return null;
  return (
    <Warning>
      <p>{t("partner.extraRights")}</p>
      <ul className="mt-1 list-disc pl-5">
        {channel.extra_rights.map((r) => (
          <li key={r}>{rightLabel(t, r)}</li>
        ))}
      </ul>
    </Warning>
  );
}

// Partner is the one screen of a partner: their channels where the bot is an admin, each with a
// warning listing the rights the bot has beyond what it needs, then a button that adds the bot
// to a channel and a refresh that reads the channels and rights again.
export default function Partner({ me }: { me: Me }) {
  const t = useT();
  const list = useLoad(() => must(api.GET("/api/channels", { params: { query: { as_partner: true } } })), []);

  let channels;
  if (list.loading && !list.data) channels = <Loading />;
  else if (list.error || !list.data) channels = <ErrorText error={list.error} onRetry={list.reload} />;
  else if (list.data.channels.length > 0) {
    channels = (
      <Card className="overflow-hidden">
        <ul aria-label={t("partner.channels")} className="divide-y divide-hint/20">
          {list.data.channels.map((c) => (
            <li key={c.id}>
              <ChannelRow channel={c} />
              <RightsWarning channel={c} />
            </li>
          ))}
        </ul>
      </Card>
    );
  }

  return (
    <Section>
      <Title>{t("partner.title")}</Title>
      {channels}
      <TitledCard icon={<Handshake className="size-5" />} title={t("partner.connect")}>
        <p>{t("partner.why")}</p>
        <p className="flex items-center gap-2 font-semibold">
          <CircleX aria-hidden className="size-5 shrink-0 text-danger" />
          {t("partner.noRights")}
        </p>
        <p className="text-sm text-hint">{t("partner.whyAdmin")}</p>
        <p>{t("partner.steps")}</p>
        <Button variant="primary" full onClick={() => openLink(me.partner_link)}>
          <UserPlus aria-hidden className="size-5" />
          {t("partner.add")}
        </Button>
        <Button variant="outline" full busy={list.loading} onClick={list.reload}>
          {!list.loading && <RefreshCw aria-hidden className="size-5" />}
          {t("partner.refresh")}
        </Button>
      </TitledCard>
    </Section>
  );
}
