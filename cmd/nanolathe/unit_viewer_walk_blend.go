package main

import "math"

// The Move preview shares Enhanced's held-axis policy (GPU design §13.5).
// Its detached VM records every completed tick, including catch-up updates.
func (m *unitViewerModel) walkPreviewActive() bool {
	return m.walkEnabled && m.anim != nil && m.anim.action == unitViewerMoving && !m.anim.invalid
}

func (m *unitViewerModel) prepareWalkPreview() {
	if !m.walkPreviewActive() || m.walkAnim == m.anim {
		return
	}
	m.walkAnim = m.anim
	m.walkHistory.Reset()
	a := m.anim
	m.walkCurrent = append(m.walkCurrent[:0], a.poses()...)
	m.walkPrevious = append(m.walkPrevious[:0], m.walkCurrent...)
	m.walkHistory.Record(uint32(a.ticks), m.walkCurrent)
	a.afterTick = func() {
		m.walkPrevious = append(m.walkPrevious[:0], m.walkCurrent...)
		m.walkCurrent = append(m.walkCurrent[:0], a.poses()...)
		m.walkHistory.Record(uint32(a.ticks), m.walkCurrent)
	}
}

func (m *unitViewerModel) walkPreviewFractionChanged(dt float64) bool {
	return m.walkPreviewActive() && m.walkSmooth && !m.anim.stopped && dt > 0 && !math.IsNaN(dt) && !math.IsInf(dt, 0)
}
