package relay

import "time"

// HostedMatchProgress is the relay's latest report on a running hosted match:
// how far every seat has acknowledged and how far the relay has compared
// their reports (DESIGN_MULTIPLAYER §16.5.2). It is host diagnostics only;
// nothing in it reaches the simulation.
type HostedMatchProgress struct {
	// Agreed is the last tick whose ended bits, and at every 30th tick unit
	// checksums, the relay has compared across every playing seat.
	Agreed uint32
	// Seats holds one entry per slot, in slot order.
	Seats []HostedSeatProgress
}

// HostedSeatProgress is one slot in a HostedMatchProgress.
type HostedSeatProgress struct {
	Playing bool          // still connected and compared; false once it left
	Final   bool          // its result is final, so it may leave
	Acked   uint32        // the last tick it acknowledged
	RTT     time.Duration // the relay's latest ping round trip; 0 when unmeasured
}

// LocalTraffic counts a client's relay messages since it connected, and their
// bytes including each message's length prefix.
type LocalTraffic struct {
	MessagesIn, MessagesOut uint64
	BytesIn, BytesOut       uint64
}

// Progress returns the relay's latest report on the running match, and false
// before the first one or on a relay that sends none.
func (c *LocalClient) Progress() (HostedMatchProgress, bool) {
	// TODO(mp-browser): stub until the relay unit implements the report.
	return HostedMatchProgress{}, false
}

// Traffic returns the client's relay message counts so far.
func (c *LocalClient) Traffic() LocalTraffic {
	// TODO(mp-browser): stub until the relay unit implements the counters.
	return LocalTraffic{}
}
