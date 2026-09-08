//go:build !linux && !windows

package backup

import "errors"

func AvailableBytes(string) (uint64, error) { return 0, errors.New("BACKUP_HOST_UNSUPPORTED") }
func privateDirectory(string) error         { return errors.New("BACKUP_HOST_UNSUPPORTED") }
