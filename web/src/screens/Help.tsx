import { Check, Copy, Handshake, Megaphone, Plus } from "lucide-react";
import { useState, type ReactNode } from "react";
import type { Me } from "../api/client";
import { useT } from "../i18n";
import { openLink } from "../telegram";
import { Button, inputClass, Section, Title, TitledCard, useNotify } from "../ui";

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

// PartnerLink copies the partner link. Where the browser refuses the copy, the link shows in a
// read-only field for the user to copy by hand.
function PartnerLink({ link }: { link: string }) {
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

// Help explains the bot in two sections, for the admins of channels that run giveaways and for
// partner channels, each with the link that adds the bot the way that section needs.
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
        <Button variant="primary" full onClick={() => openLink(me.add_link)}>
          <Plus aria-hidden className="size-5" />
          {t("help.add")}
        </Button>
      </TitledCard>

      <TitledCard icon={<Handshake className="size-5" />} title={t("help.partners")}>
        <p>{t("help.partner1")}</p>
        <p className="text-hint">{t("help.partner2")}</p>
        <PartnerLink link={me.partner_link} />
      </TitledCard>
    </Section>
  );
}
