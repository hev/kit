package layer

// Request sends a native Layer wire request using the client's shared transport,
// authentication, timeout and error handling. Callers own the request schema.
func (c *Client) Request(method, path string, body, out any) error {
	return c.do(method, path, body, out)
}
