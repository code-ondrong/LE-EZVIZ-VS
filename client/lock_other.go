//go:build !unix

package client

// lockFile does nothing where flock isn't available, processes sharing a session file may then log in at the same time
func lockFile(path string) (func(), error) {
	return func() {}, nil
}
