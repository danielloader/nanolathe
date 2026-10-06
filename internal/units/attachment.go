package units

// AttachmentObserver is the movement owner's synchronous spatial projection
// of an accepted attachment change [04 R-COLL-01 §11]. Implementations must
// be pointer owners: identity protects a replacement binding during teardown.
type AttachmentObserver interface {
	AttachmentChanged(*Unit)
}

// SetAttachmentObserver installs the world's sole attachment-index owner.
// Binding does not replay existing relationships or change list order.
func (w *World) SetAttachmentObserver(observer AttachmentObserver) {
	if w != nil {
		w.attachmentObserver = observer
	}
}

// ClearAttachmentObserver removes only the expected pointer owner.
func (w *World) ClearAttachmentObserver(expected AttachmentObserver) {
	if w != nil && w.attachmentObserver == expected {
		w.attachmentObserver = nil
	}
}

// NotifyAttachmentChanged runs after the relationship writes and before the
// requested mover-mode write. Nil means an uncomposed host context, not a
// retail attachment-index implementation [04 R-COLL-01 §11].
func (w *World) NotifyAttachmentChanged(u *Unit) {
	if w != nil && w.attachmentObserver != nil && u != nil {
		w.attachmentObserver.AttachmentChanged(u)
	}
}
