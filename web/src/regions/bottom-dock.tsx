import styles from "./bottom-dock.module.css";
import { Plus } from "lucide-react";
import Button from "@/components/arc/button/button";
import { useApp } from "@/state/app-state";

export default function BottomDock() {
  const { setAddUrlOpen } = useApp();

  return (
    <div className={styles.dock}>
      <Button variant="primary" onClick={() => setAddUrlOpen(true)}>
        <Plus size={16} strokeWidth={2} aria-hidden="true" />
        Add URL
      </Button>
    </div>
  );
}
