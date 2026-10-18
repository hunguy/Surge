package progress

import (
	"testing"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestBitmap_AllChunksComplete(t *testing.T) {
	const chunkSize = int64(4096)
	const totalSize = int64(chunkSize * 3)

	t.Run("no bitmap means single stream", func(t *testing.T) {
		var b BitmapTracker
		if !b.AllChunksComplete(totalSize) {
			t.Fatal("untracked download should report complete")
		}
	})

	t.Run("partial coverage is incomplete", func(t *testing.T) {
		var b BitmapTracker
		b.InitBitmap(totalSize, chunkSize)
		b.UpdateChunkStatus(totalSize, 0, chunkSize, types.ChunkCompleted)
		if b.AllChunksComplete(totalSize) {
			t.Fatal("one written chunk of three must not report complete")
		}
	})

	t.Run("full coverage is complete", func(t *testing.T) {
		var b BitmapTracker
		b.InitBitmap(totalSize, chunkSize)
		b.UpdateChunkStatus(totalSize, 0, totalSize, types.ChunkCompleted)
		if !b.AllChunksComplete(totalSize) {
			t.Fatal("fully written file should report complete")
		}
	})

	t.Run("unknown total size is incomplete", func(t *testing.T) {
		var b BitmapTracker
		b.InitBitmap(totalSize, chunkSize)
		b.UpdateChunkStatus(totalSize, 0, totalSize, types.ChunkCompleted)
		if b.AllChunksComplete(0) {
			t.Fatal("tracked bitmap without a known total must not report complete")
		}
	})
}
