package progress

import "testing"

// A restarted download can have its TotalSize reset to 0 while the bitmap is
// still populated. GetBitmapSnapshot must not panic on the resulting mismatch.
func TestGetBitmapSnapshot_StaleTotalSizeDoesNotPanic(t *testing.T) {
	ps := New("stale", 1000)
	ps.InitBitmap(1000, 100)
	ps.SetTotalSize(0)

	bitmap, width, _, _, progress := ps.GetBitmap()
	if width != 10 || len(bitmap) != 3 {
		t.Fatalf("got width=%d len=%d, want width=10 len=3", width, len(bitmap))
	}
	if len(progress) != 10 {
		t.Fatalf("progress len = %d, want 10", len(progress))
	}
}
