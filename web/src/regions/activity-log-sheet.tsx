import styles from "./activity-log-sheet.module.css";
import { useMemo } from "react";
import BottomSheet from "@/components/arc/bottom-sheet/bottom-sheet";
import Button from "@/components/arc/button/button";
import { Timeline, type TimelineEvent } from "@/components/arc/timeline/timeline";
import { useActivityLog, type LogLevel } from "@/state/activity-log";

/** Arc's timeline has no warning tone. Say the outcome in the line, and let red carry trouble. */
const TONES: Record<LogLevel, TimelineEvent["tone"]> = {
  info: "neutral",
  success: "success",
  warning: "danger",
};

export default function ActivityLogSheet() {
  const { entries, logOpen, setLogOpen, clear } = useActivityLog();

  const events = useMemo<TimelineEvent[]>(
    () => entries.map(entry => ({ id: entry.id, at: entry.at, title: entry.message, tone: TONES[entry.level] })),
    [entries],
  );

  // The mock clock sits just past the newest line, so the newest day reads as today.
  const now = entries.length > 0 ? entries[entries.length - 1].at + 1_000 : 0;

  return (
    <BottomSheet
      open={logOpen}
      onOpenChange={setLogOpen}
      title="Server activity"
      description="Mock log lines. Nothing is fetched."
      detents={[0.5, 0.92]}
    >
      <div className={styles.body}>
        <div className={styles.toolbar}>
          <Button variant="secondary" size="sm" onClick={clear} disabled={entries.length === 0}>
            Clear log
          </Button>
        </div>
        {entries.length === 0 ? (
          <p className={styles.empty}>No activity yet.</p>
        ) : (
          <Timeline events={events} now={now} label="Server activity" maxHeight="52vh" />
        )}
      </div>
    </BottomSheet>
  );
}
