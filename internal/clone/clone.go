package clone

// Cloner performs copy-on-write directory clones.
type Cloner interface {
	// Clone performs a CoW clone from src to dst.
	Clone(src, dst string) error
}

// ProgressEvent is emitted by progress-capable cloners.
type ProgressEvent struct {
	Copied int
	Total  int
	Phase  string
}

// ProgressFunc handles progress events.
type ProgressFunc func(ProgressEvent)

// ProgressCloner is implemented by cloners that can emit clone progress.
type ProgressCloner interface {
	CloneWithProgress(src, dst string, onProgress ProgressFunc) error
}
