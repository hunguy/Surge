import styles from "./header-bar.module.css";
import { Settings } from "lucide-react";
import Badge from "@/components/arc/badge/badge";
import Button from "@/components/arc/button/button";
import { useMockApp } from "@/state/mock-app-state";
import { useActivityLog } from "@/state/activity-log";

export default function HeaderBar() {
  const { connection, setConnectionOpen } = useMockApp();
  const { setLogOpen } = useActivityLog();

  const dot = <span className={styles.dot} aria-hidden="true" />;

  return (
    <header className={styles.header}>
      <h1 className={styles.title}>Surge</h1>
      <div className={styles.actions}>
        {/* The status pill opens the activity log. */}
        <button type="button" className={styles.status} onClick={() => setLogOpen(true)}>
          <Badge tone={connection.connected ? "success" : "danger"} icon={dot}>
            {connection.connected ? "Connected" : "Disconnected"}
          </Badge>
        </button>
        <Button variant="ghost" size="sm" aria-label="Server settings" onClick={() => setConnectionOpen(true)}>
          <Settings size={18} strokeWidth={1.75} aria-hidden="true" />
        </Button>
      </div>
    </header>
  );
}
