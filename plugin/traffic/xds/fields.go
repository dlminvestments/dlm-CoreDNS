package xds

func (c *Client) Nonce() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nonce
}

func (c *Client) SetNonce(n string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nonce = n
}

func (c *Client) Assignments() *assignment {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.assignments
}

func (c *Client) SetAssignments(a *assignment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.assignments = a
}
