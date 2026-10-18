//go:build !linux && !darwin

package nativegate

import "os"

func openPrivate(string) (*os.File, error)          { return nil, ErrReceiverCall }
func openPinned(string, bool) (*os.File, error)     { return nil, ErrReceiverCall }
func platformPrivateDirectory(string) error         { return ErrReceiverCall }
func platformCreatePrivateDirectory(string) error   { return ErrReceiverCall }
func platformSyncDirectory(string) error            { return ErrReceiverCall }
func platformPublishExclusive(string, []byte) error { return ErrReceiverCall }

func privateFile(string) error { return ErrReceiverCall }
