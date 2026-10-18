import { AppProvider } from "@/state/app-state";
import DownloadManagerPage from "@/pages/download-manager-page";

export default function App() {
  return (
    <AppProvider>
      <DownloadManagerPage />
    </AppProvider>
  );
}
