package feast

import "fmt"

func (c *Client) debugf(format string, args ...any) {
	if c == nil || !c.opts.Debug {
		return
	}
	if len(args) == 0 {
		fmt.Println(format)
		return
	}
	fmt.Printf(format+"\n", args...)
}

func (c *Client) debugActionf(action, format string, args ...any) {
	prefix := "[" + action + "] "
	c.debugf(prefix+format, args...)
}
