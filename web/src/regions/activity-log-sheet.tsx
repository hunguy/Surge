import styles from "./activity-log-sheet.module.css";
import { useMemo } from "react";
import BottomSheet from "@/components/arc/bottom-sheet/bottom-sheet";
import Button from "@/components/arc/button/button";
import { Timeline, type TimelineEvent } from "@/components/arc/timeline/timeline";
import { useApp } from "@/state/app-state";

/** Arc's timeline has no warning tone. Say the outcome in the line, and let red carry trouble. */
const TONES: Record<string, TimelineEvent["tone"]> = {
  info: "neutral",
  success: "success",
  warning: "danger",
};

export default function ActivityLogSheet() {
  const { log, logOpen, setLogOpen, clearLog } = useApp();

  const events = useMemo<TimelineEvent[]>(
    () => log.map(entry => ({ id: entry.id, at: entry.at, title: entry.message, tone: TONES[entry.level] ?? "neutral" })),
    [log],
  );

  // The newest line is the reference clock: real wall-clock seconds, and stable
  // across renders in a way Date.now() during render is not.
  const now = log.length > 0 ? log[log.length - 1]!.at : 0;

  return (
    <BottomSheet
      open={logOpen}
      onOpenChange={setLogOpen}
      title="Server activity"
      description="Server messages and local connection events."
      detents={[0.5, 0.92]}
    >
      <div className={styles.body}>
        <div className={styles.toolbar}>
          <Button variant="secondary" size="sm" onClick={clearLog} disabled={log.length === 0}>
            Clear log
          </Button>
        </div>
        {log.length === 0 ? (
          <p className={styles.empty}>No activity yet.</p>
        ) : (
          <Timeline events={events} now={now} label="Server activity" maxHeight="52vh" />
        )}
      </div>
    </BottomSheet>
  );
}
