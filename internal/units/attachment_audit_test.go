package units

import "testing"

type attachmentOwnerProbe struct {
	calls   int
	carrier uint32
	mode    uint8
}

func (o *attachmentOwnerProbe) AttachmentChanged(u *Unit) {
	o.calls++
	o.carrier = uint32(u.Attachment.Carrier)
	o.mode = u.Move.Mode
}
func TestAttachmentObserverIdentityAndNoReplay(t *testing.T) {
	w := &World{}
	u := &Unit{}
	a, b := &attachmentOwnerProbe{}, &attachmentOwnerProbe{}
	w.SetAttachmentObserver(a)
	w.NotifyAttachmentChanged(u)
	w.SetAttachmentObserver(b)
	w.ClearAttachmentObserver(a)
	w.NotifyAttachmentChanged(u)
	if a.calls != 1 || b.calls != 1 {
		t.Fatalf("replacement owner lost: %d %d", a.calls, b.calls)
	}
	w.SetAttachmentObserver(b)
	if b.calls != 1 {
		t.Fatal("bind replayed state")
	}
	w.ClearAttachmentObserver(b)
	w.NotifyAttachmentChanged(u)
	if b.calls != 1 {
		t.Fatal("expected owner not cleared")
	}
}
