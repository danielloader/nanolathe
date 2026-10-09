package relay

import "time"

type localPendingCommand struct {
	command LocalCommand
	readyAt time.Time
}

// The same fixed delay applies to both seats, preserving accepted stream order.
// Delayed entries retain the ordinary count/byte bounds (DESIGN_MULTIPLAYER §16.4.3).
type localCommandQueue struct {
	entries []localPendingCommand
	bytes   int
	sealed  uint64
}

func (q *localCommandQueue) release(now time.Time) []LocalCommand {
	var commands []LocalCommand
	for _, entry := range q.entries {
		if now.Before(entry.readyAt) {
			break
		}
		commands = append(commands, entry.command)
		q.bytes -= len(entry.command.Payload)
		q.sealed = entry.command.Position
	}
	// Discard payload references even while the remaining queue shares storage.
	clear(q.entries[:len(commands)])
	q.entries = q.entries[len(commands):]
	return commands
}
