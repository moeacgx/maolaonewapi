//go:build !windows

package service

func validateRequestArchiveLocalPlatformPath(_ string) error {
	return nil
}
