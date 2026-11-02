import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { Failure, type Giveaway } from "./api/client";
import { useT } from "./i18n";

export function Loading() {
  const t = useT();
  return (
    <Box role="status" aria-live="polite" sx={{ display: "flex", justifyContent: "center", py: 6 }}>
      <CircularProgress aria-label={t("app.loading")} />
    </Box>
  );
}

// ErrorText shows the text for a failed call, with a retry when given.
export function ErrorText({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const t = useT();
  const code = error instanceof Failure ? error.code : "internal";
  return (
    <Alert
      severity="error"
      action={
        onRetry && (
          <Button color="inherit" size="small" onClick={onRetry}>
            {t("app.retry")}
          </Button>
        )
      }
    >
      {t(`error.${code}`)}
    </Alert>
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
    <Dialog open={props.open} onClose={props.onClose} aria-labelledby="confirm-title" fullWidth maxWidth="xs">
      <DialogTitle id="confirm-title">{props.title}</DialogTitle>
      <DialogContent>
        <DialogContentText>{props.text}</DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={props.onClose}>{t("app.cancel")}</Button>
        <Button
          onClick={props.onConfirm}
          variant="contained"
          color={props.danger ? "error" : "primary"}
          disabled={props.busy}
        >
          {props.confirm}
        </Button>
      </DialogActions>
    </Dialog>
  );
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

// statusColor is the chip color of a giveaway status.
export function statusColor(status: Giveaway["status"]) {
  switch (status) {
    case "active":
      return "success";
    case "review":
      return "warning";
    case "cancelled":
      return "default";
    default:
      return "info";
  }
}

export function Section({ children }: { children: ReactNode }) {
  return <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>{children}</Box>;
}
