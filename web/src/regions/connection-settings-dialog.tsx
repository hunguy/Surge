import { useState, type FormEvent } from "react";
import styles from "./connection-settings-dialog.module.css";
import { X } from "lucide-react";
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog";
import ThemeSwitch from "@/components/arc/theme-switch/theme-switch";
import { Input } from "@/components/arc/input/input";
import { PasswordField } from "@/components/arc/password-field/password-field";
import Button from "@/components/arc/button/button";
import Alert from "@/components/arc/alert/alert";
import { useApp } from "@/state/app-state";
import { hostOf, savedNameFor } from "@/regions/server-label";
import { useTheme } from "@/regions/use-theme";
import type { SavedConnection } from "@/types";

/** The dialog body unmounts when closed, so it opens on the current saved values. */
export default function ConnectionSettingsDialog() {
  const { connection, savedConnections, connectionOpen, setConnectionOpen, saveConnection, removeSavedConnection, disconnect } =
    useApp();
  const { theme, setTheme } = useTheme();

  // Reloading a tile starts a form that is already named, so opening on a server you have
  // saved again and pressing Save overwrites that tile instead of adding a second one.
  const currentName = savedNameFor(connection, savedConnections) ?? "";

  return (
    <Dialog open={connectionOpen} onOpenChange={setConnectionOpen}>
      <DialogContent
        title="Server settings"
        description="Saved in this browser, and sent only to the server above."
      >
        {connectionOpen && (
          <SettingsForm
            key={`${connection.serverUrl}\n${connection.token}`}
            initialName={currentName}
            initialUrl={connection.serverUrl}
            initialToken={connection.token}
            saved={savedConnections}
            connected={connection.connected}
            theme={theme}
            onThemeChange={setTheme}
            onSave={saveConnection}
            onRemoveSaved={removeSavedConnection}
            onDisconnect={disconnect}
            onClose={() => setConnectionOpen(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function SettingsForm({
  initialName,
  initialUrl,
  initialToken,
  saved,
  connected,
  theme,
  onThemeChange,
  onSave,
  onRemoveSaved,
  onDisconnect,
  onClose,
}: {
  initialName: string;
  initialUrl: string;
  initialToken: string;
  saved: SavedConnection[];
  connected: boolean;
  theme: "light" | "dark";
  onThemeChange: (theme: "light" | "dark") => void;
  onSave: (name: string, serverUrl: string, token: string) => void;
  onRemoveSaved: (name: string) => void;
  onDisconnect: () => void;
  onClose: () => void;
}) {
  const [name, setName] = useState(initialName);
  const [serverUrl, setServerUrl] = useState(initialUrl);
  const [token, setToken] = useState(initialToken);
  const [error, setError] = useState<string | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const url = serverUrl.trim();
    const label = name.trim();
    if (!label) {
      setError("Enter a name for this connection before saving.");
      return;
    }
    if (!url) {
      setError("Enter the server URL before saving.");
      return;
    }
    if (!token.trim()) {
      setError("Enter the access token before saving.");
      return;
    }
    onSave(label, url, token.trim());
    onClose();
  };

  // A tile fills the form rather than connecting, so the address can be checked before it is used.
  const load = (entry: SavedConnection) => {
    setName(entry.name);
    setServerUrl(entry.serverUrl);
    setToken(entry.token);
    setError(null);
  };

  return (
    <>
      <form className={styles.form} onSubmit={submit}>
        <Input
          label="Name"
          value={name}
          placeholder="Home server"
          autoComplete="off"
          spellCheck={false}
          onChange={event => setName(event.target.value)}
        />
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
      {saved.length > 0 && (
        <section className={styles.saved}>
          <h3 className={styles.savedTitle}>Saved connections</h3>
          <ul className={styles.grid}>
            {saved.map(entry => (
              <li key={entry.name} className={styles.tile}>
                <button type="button" className={styles.tileLoad} onClick={() => load(entry)}>
                  <span className={styles.tileName}>{entry.name}</span>
                  <span className={styles.tileHost}>{hostOf(entry.serverUrl)}</span>
                </button>
                {/* No confirmation: the address stays in the form, so saving it again undoes this. */}
                <button
                  type="button"
                  className={styles.tileRemove}
                  aria-label={`Remove ${entry.name}`}
                  onClick={() => onRemoveSaved(entry.name)}
                >
                  <X size={12} strokeWidth={2} aria-hidden="true" />
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}
