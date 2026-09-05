package cmd

import (
	"github.com/saltyorg/sb-go/facts"
	"time"
)

func withFactFileLockTimeout(path string, timeout time.Duration, action func() error) error {
	return facts.WithFileLockTimeout(path, timeout, action)
}
