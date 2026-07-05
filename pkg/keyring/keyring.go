package keyring

import (
	"flag"

	"github.com/zalando/go-keyring"
)

var IsMocked = false

// MockInit enables in-memory mocking for tests.
func MockInit() {
	keyring.MockInit()
	IsMocked = true
}

func Set(service, user, password string) error {
	enforceSandbox()
	return keyring.Set(service, user, password)
}

func Get(service, user string) (string, error) {
	enforceSandbox()
	return keyring.Get(service, user)
}

func Delete(service, user string) error {
	enforceSandbox()
	return keyring.Delete(service, user)
}

func enforceSandbox() {
	if isTest() && !IsMocked {
		panic("CRITICAL SAFETY BLOCK: Attempted to interact with real OS keyring during Go tests without calling keyring.MockInit()! Sandboxing is missing.")
	}
}

func isTest() bool {
	return flag.Lookup("test.v") != nil
}
