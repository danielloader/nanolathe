package main

func nlCardValueText(c nlCard, d *nlDraft) string {
	if c.valueText != nil {
		return c.valueText(d)
	}
	return c.steps[max(0, min(len(c.steps)-1, c.get(d)))]
}

func nlPartValueText(p nlPart, d *nlDraft) string {
	if p.valueText != nil {
		return p.valueText(d)
	}
	return p.steps[p.get(d)]
}

// cardUnavailable describes why the current draft cannot use a preference.
// Keep stored choices intact when changing modes: only editing and comparison
// are disabled (DESIGN_INTERFACE_HUD_INPUT §3.17).
func (s *nlScreen) cardUnavailable(c nlCard) string {
	if c.enhanced && s.draft.pres.Renderer == "classic" {
		return "Requires the Enhanced renderer."
	}
	if c.key == "softshadows" && (s.draft.shadows == 0 || s.draft.vehicleShadows == 0) {
		return "Enable unit shadows in Visual Options."
	}
	if c.key == "content" {
		if g := s.shell(); g != nil && g.cs != nil && g.cs.manualRoots {
			return "Content is fixed by the command-line asset stack."
		}
	}
	if c.key == "unitlimit" {
		if g := s.shell(); g != nil && g.opts.UnitLimit != 0 {
			return "Unit limit is set by --unit-limit for this run."
		}
	}
	return configurationUnavailable(c.key, s.draft.gameplay, s.draft.pres)
}

func (s *nlScreen) partUnavailable(c nlCard, p nlPart) string {
	if reason := s.cardUnavailable(c); reason != "" {
		return reason
	}
	d := &s.draft
	switch p.key {
	case "groundLightStrength":
		if d.pres.GroundLight == 0 {
			return "Enable Ground light in Lighting."
		}
	case "blastRingStrength":
		if d.pres.BlastRings == 0 {
			return "Enable Blast rings."
		}
	case "shadowSoftness":
		if d.pres.SoftShadows == 0 {
			return "Enable Soft shadows."
		}
	case "weaponGlowStrength", "explosionGlowStrength":
		if d.glow == 0 || d.glowStrength <= 0 {
			return "Enable Overall glow."
		}
	case "nanoGlowStrength":
		if (d.glow == 0 || d.glowStrength <= 0) && d.pres.ModelLight == 0 && (d.pres.GroundLight == 0 || d.pres.GroundLightStrength <= 0) {
			return "Enable glow, unit light or ground light."
		}
	}
	return ""
}

func (s *nlScreen) compareAvailable(c nlCard) bool {
	if c.compare == nil || s.cardUnavailable(c) != "" {
		return false
	}
	if c.kind == nlGroup && s.partUnavailable(c, c.parts[s.selectedPart(&c)]) != "" {
		return false
	}
	_, ok := c.compare(&s.draft, c.get(&s.draft))
	return ok
}

func (s *nlScreen) setPart(c nlCard, i, value int) {
	if i < 0 || i >= len(c.parts) || s.partUnavailable(c, c.parts[i]) != "" {
		return
	}
	p := c.parts[i]
	if value < 0 || value >= len(p.steps) {
		return
	}
	s.partSel[c.key] = i
	next := s.draft
	p.set(&next, value)
	s.setCardDraft(c, next)
}
