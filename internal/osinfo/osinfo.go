package osinfo

// Client answers OS-specific questions about running processes.
// Construct one via the platform-specific New function (e.g. NewWithFS on Linux).
// The zero value is not usable; always use a constructor.
type Client struct {
	foregroundCommand func(pid int) (string, error)
	foregroundCWD     func(pid int) (string, error)
}

// ForegroundCommand returns the command name of the foreground process
// associated with the shell running at pid.
func (c *Client) ForegroundCommand(pid int) (string, error) {
	return c.foregroundCommand(pid)
}

// ForegroundCWD returns the current working directory of the process at pid.
func (c *Client) ForegroundCWD(pid int) (string, error) {
	return c.foregroundCWD(pid)
}
