import { Check, Handshake, Megaphone } from "lucide-react";
import type { ReactNode } from "react";
import type { Me } from "../api/client";
import { useT } from "../i18n";
import { PartnerLink, Section, Title, TitledCard } from "../ui";

// Step is one numbered step of the admin section.
function Step({ n, children }: { n: number; children: ReactNode }) {
  return (
    <li className="flex gap-3">
      <span aria-hidden className="flex size-7 shrink-0 items-center justify-center rounded-full bg-accent text-sm font-semibold text-on-accent">
        {n}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-2 pt-0.5">{children}</div>
    </li>
  );
}

function Right({ name, why }: { name: string; why: string }) {
  return (
    <li className="flex gap-2">
      <Check aria-hidden className="mt-0.5 size-5 shrink-0 text-ok" />
      <span>
        <b className="font-medium">{name}</b> <span className="text-hint">— {why}</span>
      </span>
    </li>
  );
}

// Help explains the bot in two sections, for the admins of the primary channel, where giveaways
// run, and for partner channels, with the link that adds the bot as a partner.
export default function Help({ me }: { me: Me }) {
  const t = useT();
  return (
    <Section>
      <Title>{t("help.title")}</Title>
      <p className="text-hint">{t("help.intro")}</p>

      <TitledCard icon={<Megaphone className="size-5" />} title={t("help.admins")}>
        <ol className="flex flex-col gap-4">
          <Step n={1}>
            <p>{t("help.step1")}</p>
            <ul className="flex flex-col gap-1.5">
              <Right name={t("help.rightPost")} why={t("help.rightPostWhy")} />
              <Right name={t("help.rightEdit")} why={t("help.rightEditWhy")} />
            </ul>
            <p className="text-sm text-hint">{t("help.rightsOff")}</p>
          </Step>
          <Step n={2}>
            <p>{t("help.step2")}</p>
          </Step>
          <Step n={3}>
            <p>{t("help.step3")}</p>
          </Step>
        </ol>
      </TitledCard>

      <TitledCard icon={<Handshake className="size-5" />} title={t("help.partners")}>
        <p>{t("help.partner1")}</p>
        <p className="text-hint">{t("help.partner2")}</p>
        <PartnerLink link={me.partner_link} />
      </TitledCard>
    </Section>
  );
}
