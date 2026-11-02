import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import List from "@mui/material/List";
import ListItemButton from "@mui/material/ListItemButton";
import ListItemText from "@mui/material/ListItemText";
import ListSubheader from "@mui/material/ListSubheader";
import SvgIcon from "@mui/material/SvgIcon";
import Paper from "@mui/material/Paper";
import Tab from "@mui/material/Tab";
import Tabs from "@mui/material/Tabs";
import Typography from "@mui/material/Typography";
import { useState } from "react";
import { api, must, type Giveaway } from "../api/client";
import { useT, type T } from "../i18n";
import { formatEnd } from "../time";
import { ErrorText, Loading, Section, statusColor, useLoad } from "../ui";

const pageSize = 25;

const plusPath = <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" />;

function GiveawayRow({ g, t, onOpen }: { g: Giveaway; t: T; onOpen: () => void }) {
  return (
    <ListItemButton onClick={onOpen} sx={{ gap: 1, minHeight: 64 }}>
      <ListItemText
        primary={g.button_text}
        secondary={`${g.ends_at ? t("list.ends", { time: formatEnd(g.ends_at) }) : t("list.noEnd")} · ${t(
          "list.participants",
          { n: g.participants },
        )}`}
        slotProps={{ primary: { noWrap: true } }}
      />
      <Chip size="small" label={t(`status.${g.status}`)} color={statusColor(g.status)} />
    </ListItemButton>
  );
}

function Running({ onOpen, onReview, onNew }: Props) {
  const t = useT();
  const list = useLoad(() => must(api.GET("/api/giveaways")), []);

  if (list.loading && !list.data) return <Loading />;
  if (list.error || !list.data) return <ErrorText error={list.error} onRetry={list.reload} />;

  const { giveaways, review } = list.data;
  return (
    <Section>
      {review.length > 0 && (
        <Paper>
          <List subheader={<ListSubheader sx={{ bgcolor: "transparent" }}>{t("list.review")}</ListSubheader>}>
            {review.map((g) => (
              <GiveawayRow key={g.id} g={g} t={t} onOpen={() => onReview(g.id)} />
            ))}
          </List>
        </Paper>
      )}
      {giveaways.length > 0 ? (
        <Paper>
          <List subheader={<ListSubheader sx={{ bgcolor: "transparent" }}>{t("list.mine")}</ListSubheader>}>
            {giveaways.map((g) => (
              <GiveawayRow key={g.id} g={g} t={t} onOpen={() => onOpen(g.id)} />
            ))}
          </List>
        </Paper>
      ) : (
        <Typography color="text.secondary" sx={{ textAlign: "center", py: 3 }}>
          {t("list.empty")}
        </Typography>
      )}
      <Button variant="contained" size="large" onClick={onNew} startIcon={<SvgIcon>{plusPath}</SvgIcon>} fullWidth>
        {t("list.new")}
      </Button>
    </Section>
  );
}

function Finished({ onOpen, onReview }: Props) {
  const t = useT();
  const [pages, setPages] = useState(1);
  const list = useLoad(
    () => must(api.GET("/api/giveaways/finished", { params: { query: { limit: pages * pageSize } } })),
    [pages],
  );

  if (list.loading && !list.data) return <Loading />;
  if (list.error || !list.data) return <ErrorText error={list.error} onRetry={list.reload} />;
  if (list.data.giveaways.length === 0) {
    return (
      <Typography color="text.secondary" sx={{ textAlign: "center", py: 3 }}>
        {t("list.finishedEmpty")}
      </Typography>
    );
  }

  return (
    <Section>
      <Paper>
        <List>
          {list.data.giveaways.map((g) => (
            <GiveawayRow
              key={g.id}
              g={g}
              t={t}
              onOpen={() => (g.status === "review" ? onReview(g.id) : onOpen(g.id))}
            />
          ))}
        </List>
      </Paper>
      {list.data.more && (
        <Button onClick={() => setPages((p) => p + 1)} disabled={list.loading}>
          {t("list.more")}
        </Button>
      )}
    </Section>
  );
}

interface Props {
  onOpen: (id: number) => void;
  onReview: (id: number) => void;
  onNew: () => void;
}

export default function GiveawayList(props: Props & { tab: number; onTab: (tab: number) => void }) {
  const t = useT();
  return (
    <Section>
      <Tabs value={props.tab} onChange={(_, v: number) => props.onTab(v)} variant="fullWidth">
        <Tab label={t("list.running")} id="tab-0" aria-controls="tabpanel-0" />
        <Tab label={t("list.finished")} id="tab-1" aria-controls="tabpanel-1" />
      </Tabs>
      <div role="tabpanel" id={`tabpanel-${props.tab}`} aria-labelledby={`tab-${props.tab}`}>
        {props.tab === 0 ? <Running {...props} /> : <Finished {...props} />}
      </div>
    </Section>
  );
}
