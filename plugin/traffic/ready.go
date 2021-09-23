package traffic

// Ready implements the ready.Readiness interface.
func (t *Traffic) Ready() bool { return t.c.HasSynced() }
