package xds

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

func (c *Client) Version(typeURL string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version[typeURL]
}

func (c *Client) SetVersion(typeURL, a string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version[typeURL] = a
}

func (c *Client) Nonce(typeURL string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nonce[typeURL]
}

func (c *Client) SetNonce(typeURL, n string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nonce[typeURL] = n
}
