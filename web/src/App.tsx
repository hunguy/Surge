import { MockAppProvider } from "@/state/mock-app-state";
import { ActivityLogProvider } from "@/state/activity-log";
import DownloadManagerPage from "@/pages/download-manager-page";

export default function App() {
  return (
    <ActivityLogProvider>
      <MockAppProvider>
        <DownloadManagerPage />
      </MockAppProvider>
    </ActivityLogProvider>
  );
}
