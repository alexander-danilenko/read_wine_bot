import { Handshake, RefreshCw, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import { api, must, type Channel, type Me } from "../api/client";
import { useT, type T, type TextKey } from "../i18n";
import en from "../locales/en.json";
import { Button, Card, ChannelRow, ErrorText, Loading, PartnerLink, Section, Title, TitledCard, useLoad } from "../ui";

// rightLabel is Telegram's own name of an admin right, or the API's name for a right this app
// does not know yet.
function rightLabel(t: T, right: string): string {
  const key = `right.${right}`;
  return key in en ? t(key as TextKey) : right;
}

function Warning({ children }: { children: ReactNode }) {
  return (
    <p className="flex gap-2 px-4 pb-3 text-sm text-warn">
      <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
      <span>{children}</span>
    </p>
  );
}

// RightsWarning asks the partner to remove the rights the bot has beyond Manage chat. Null means
// the bot could not read its rights there.
function RightsWarning({ channel }: { channel: Channel }) {
  const t = useT();
  if (channel.extra_rights === null) return <Warning>{t("partner.rightsUnknown")}</Warning>;
  if (channel.extra_rights.length === 0) return null;
  return <Warning>{t("partner.extraRights", { rights: channel.extra_rights.map((r) => rightLabel(t, r)).join(", ") })}</Warning>;
}

// Partner is the one screen of a partner: their channels where the bot is an admin, each with a
// warning when the bot has more rights than it needs, then how to connect a channel and a
// refresh that reads the channels and rights again.
export default function Partner({ me }: { me: Me }) {
  const t = useT();
  const list = useLoad(() => must(api.GET("/api/channels")), []);

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
        <p>{t("partner.help")}</p>
        <PartnerLink link={me.partner_link} />
        <Button variant="outline" full busy={list.loading} onClick={list.reload}>
          {!list.loading && <RefreshCw aria-hidden className="size-5" />}
          {t("partner.refresh")}
        </Button>
      </TitledCard>
    </Section>
  );
}
