import { useState, type FormEvent } from "react";
import styles from "./connection-settings-dialog.module.css";
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog";
import ThemeSwitch from "@/components/arc/theme-switch/theme-switch";
import { Input } from "@/components/arc/input/input";
import { PasswordField } from "@/components/arc/password-field/password-field";
import Button from "@/components/arc/button/button";
import Alert from "@/components/arc/alert/alert";
import { useApp } from "@/state/app-state";
import { useTheme } from "@/regions/use-theme";

/** The dialog body unmounts when closed, so it opens on the current saved values. */
export default function ConnectionSettingsDialog() {
  const { connection, connectionOpen, setConnectionOpen, saveConnection, disconnect } = useApp();
  const { theme, setTheme } = useTheme();

  return (
    <Dialog open={connectionOpen} onOpenChange={setConnectionOpen}>
      <DialogContent
        title="Server settings"
        description="Saved in this browser, and sent only to the server above."
      >
        {connectionOpen && (
          <SettingsForm
            key={`${connection.serverUrl}\n${connection.token}`}
            initialUrl={connection.serverUrl}
            initialToken={connection.token}
            connected={connection.connected}
            theme={theme}
            onThemeChange={setTheme}
            onSave={saveConnection}
            onDisconnect={disconnect}
            onClose={() => setConnectionOpen(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function SettingsForm({
  initialUrl,
  initialToken,
  connected,
  theme,
  onThemeChange,
  onSave,
  onDisconnect,
  onClose,
}: {
  initialUrl: string;
  initialToken: string;
  connected: boolean;
  theme: "light" | "dark";
  onThemeChange: (theme: "light" | "dark") => void;
  onSave: (serverUrl: string, token: string) => void;
  onDisconnect: () => void;
  onClose: () => void;
}) {
  const [serverUrl, setServerUrl] = useState(initialUrl);
  const [token, setToken] = useState(initialToken);
  const [error, setError] = useState<string | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const url = serverUrl.trim();
    if (!url) {
      setError("Enter the server URL before saving.");
      return;
    }
    if (!token.trim()) {
      setError("Enter the access token before saving.");
      return;
    }
    onSave(url, token.trim());
    onClose();
  };

  return (
    <form className={styles.form} onSubmit={submit}>
      <Input
        label="Server URL"
        value={serverUrl}
        placeholder="http://127.0.0.1:1700"
        autoComplete="off"
        spellCheck={false}
        onChange={event => setServerUrl(event.target.value)}
      />
      <PasswordField
        label="Access token"
        value={token}
        autoComplete="off"
        spellCheck={false}
        onChange={event => setToken(event.target.value)}
      />
      <Alert open={error !== null} tone="warning" title="Check the fields" onDismiss={() => setError(null)}>
        {error}
      </Alert>
      <ThemeSwitch theme={theme} onThemeChange={onThemeChange} />
      <div className={styles.actions}>
        <Button
          type="button"
          variant="secondary"
          disabled={!connected}
          onClick={() => {
            onDisconnect();
            onClose();
          }}
        >
          Disconnect
        </Button>
        <Button type="submit" variant="primary">
          Save
        </Button>
      </div>
    </form>
  );
}
