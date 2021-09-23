package xds

// Assignments returns the current assignment map.
func (c *Client) Assignments() *assignment {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.assignments
}

// SetAssignments sets the assignment map.
func (c *Client) SetAssignments(a *assignment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.assignments = a
}

// Version returns the last version seen from the API for this typeURL.
func (c *Client) Version(typeURL string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version[typeURL]
}

// SetVersion sets the version for this typeURL.
func (c *Client) SetVersion(typeURL, a string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version[typeURL] = a
}

// Nonce returns the last nonce seen from the API for this typeURL.
func (c *Client) Nonce(typeURL string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nonce[typeURL]
}

// SetNonce sets the nonce. for this typeURL.
func (c *Client) SetNonce(typeURL, n string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nonce[typeURL] = n
}

// setSynced sets the synced boolean to true.
func (c *Client) setSynced() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.synced = true
}

// HasSynced return true if the clients has synced.
func (c *Client) HasSynced() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.synced
}
