package orders

// CheckpointMappingWord retains a callback and its original installation
// proof as one immutable copied value. It does not follow later source-slot
// edits, and neither field contributes checkpoint bytes (§16.3.66).
type CheckpointMappingWord struct {
	reader func(int32, int32) (uint16, bool)
	proof  checkpointCallbackProof[WorldQueryAdapter]
}

// CheckpointMappingWord copies the actual reader without invoking it. Only
// proof belonging to this exact adapter can travel with the copied callback;
// an ordinary installation or copied adapter still returns its actual reader.
func (b *WorldQueryAdapter) CheckpointMappingWord() CheckpointMappingWord {
	if b == nil {
		return CheckpointMappingWord{}
	}
	value := CheckpointMappingWord{reader: b.mappingWord}
	proof := b.checkpointProofs.mappingWord
	if proof.matches(b, proof.authority) {
		value.proof = proof
	}
	return value
}

// Reader returns only the copied function. An ordinary setter receiving it
// cannot recover or transfer the installation proof (§16.3.66).
func (v CheckpointMappingWord) Reader() func(int32, int32) (uint16, bool) {
	return v.reader
}

// ValidateMappingWord checks the original source against this capture's
// registered World adapter and authority. Current source-slot state belongs
// to ValidateBinding: checking it here would conflate two independent copies.
// Validation invokes no callback and never changes or refreshes either proof.
func (c *CheckpointContext) ValidateMappingWord(v CheckpointMappingWord) error {
	if c == nil {
		return bindingCheckpointError("MappingWord", "a checkpoint context")
	}
	if v.reader == nil {
		if v.proof.owner != nil || v.proof.authority != nil {
			return bindingCheckpointError("MappingWord", "no installation proof without a reader")
		}
		return nil
	}
	if c.bindings == nil || c.bindings.world == nil || !v.proof.matches(c.bindings.world, c.bindings.authority) {
		return bindingCheckpointError("MappingWord", "a copied reader from the registered World adapter and authority")
	}
	return nil
}
