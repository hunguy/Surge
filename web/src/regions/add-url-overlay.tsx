import { useState, type FormEvent } from "react";
import styles from "./add-url-overlay.module.css";
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog";
import BottomSheet from "@/components/arc/bottom-sheet/bottom-sheet";
import { Input } from "@/components/arc/input/input";
import Button from "@/components/arc/button/button";
import Alert from "@/components/arc/alert/alert";
import { useApp } from "@/state/app-state";
import { useIsPhone } from "@/regions/use-media-query";

/** Shared form body. It unmounts with the overlay, so each opening starts empty. */
function AddUrlForm({ onClose }: { onClose: () => void }) {
  const { addUrl } = useApp();
  const [url, setUrl] = useState("");
  const [savePath, setSavePath] = useState("");
  const [error, setError] = useState<string | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const value = url.trim();
    if (!value) {
      setError("Paste a link first.");
      return;
    }
    addUrl(value, savePath.trim());
    onClose();
  };

  return (
    <form className={styles.form} onSubmit={submit}>
      <Input
        label="Download URL"
        value={url}
        placeholder="https://example.com/archive.tar.zst"
        autoComplete="off"
        spellCheck={false}
        onChange={event => setUrl(event.target.value)}
      />
      <Input
        label="Save to"
        value={savePath}
        placeholder="/path/to/location"
        autoComplete="off"
        spellCheck={false}
        onChange={event => setSavePath(event.target.value)}
      />
      <Alert open={error !== null} tone="warning" title="Check the link" onDismiss={() => setError(null)}>
        {error}
      </Alert>
      <div className={styles.actions}>
        <Button type="button" variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" variant="primary">
          Queue download
        </Button>
      </div>
    </form>
  );
}

export default function AddUrlOverlay() {
  const { addUrlOpen, setAddUrlOpen } = useApp();
  const isPhone = useIsPhone();
  const close = () => setAddUrlOpen(false);
  const form = <AddUrlForm onClose={close} />;

  if (isPhone) {
    return (
      <BottomSheet
        open={addUrlOpen}
        onOpenChange={setAddUrlOpen}
        title="Add a download"
        description="Paste a link to queue it."
      >
        {form}
      </BottomSheet>
    );
  }

  return (
    <Dialog open={addUrlOpen} onOpenChange={setAddUrlOpen}>
      <DialogContent title="Add a download" description="Paste a link to queue it.">
        {form}
      </DialogContent>
    </Dialog>
  );
}
