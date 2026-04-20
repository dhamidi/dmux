package session

// Set adds or updates name=value in e.
func (e Environ) Set(name, value string) {
	e[name] = value
}

// Remove deletes name from e. It is a no-op if name is not present.
func (e Environ) Remove(name string) {
	delete(e, name)
}
