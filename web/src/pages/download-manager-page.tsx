import styles from "./download-manager-page.module.css";
import HeaderBar from "@/regions/header-bar";
import SpeedHero from "@/regions/speed-hero";
import ThroughputChart from "@/regions/throughput-chart";
import DownloadFilter from "@/regions/download-filter";
import DownloadList from "@/regions/download-list";
import AddUrlOverlay from "@/regions/add-url-overlay";
import ActivityLogSheet from "@/regions/activity-log-sheet";
import ConnectionSettingsDialog from "@/regions/connection-settings-dialog";

export default function DownloadManagerPage() {
  return (
    <div className={styles.page}>
      <HeaderBar />
      <SpeedHero />
      <ThroughputChart />
      <DownloadFilter />
      <DownloadList />
      <AddUrlOverlay />
      <ActivityLogSheet />
      <ConnectionSettingsDialog />
      <footer className={styles.version}>v{__APP_VERSION__}</footer>
    </div>
  );
}
