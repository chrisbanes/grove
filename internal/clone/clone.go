package clone

// Cloner performs copy-on-write directory clones.
type Cloner interface {
	// Clone performs a CoW clone from src to dst.
	Clone(src, dst string) error
}
